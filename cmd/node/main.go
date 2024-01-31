package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"k8s.io/apiserver/pkg/server/dynamiccertificates"
	"k8s.io/apiserver/pkg/server/options"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/virtual-kubelet/virtual-kubelet/errdefs"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	logruslogger "github.com/virtual-kubelet/virtual-kubelet/log/logrus"
	vknode "github.com/virtual-kubelet/virtual-kubelet/node"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	kubemockprovider "github.com/zhizuqiu/kube-mock/internal/provider"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

const (
	// Provider configuration defaults.
	defaultCPUCapacity    = "20"
	defaultMemoryCapacity = "100Gi"
	defaultPodCapacity    = "20"
)

var (
	buildVersion = "v1alpha1"
	k8sVersion   = "v1.29.0" // This should follow the version of k8s.io we are importing

	taintKey    = envOrDefault("VKUBELET_TAINT_KEY", "virtual-kubelet.io/provider")
	taintEffect = envOrDefault("VKUBELET_TAINT_EFFECT", string(v1.TaintEffectNoSchedule))
	taintValue  = envOrDefault("VKUBELET_TAINT_VALUE", "kube-mock")

	logLevel = "info"

	// for mock node
	kubeConfigPath  = os.Getenv("KUBECONFIG")
	cpu             = defaultCPUCapacity
	memory          = defaultMemoryCapacity
	podCapacity     = defaultPodCapacity
	clusterDomain   = "cluster.local"
	startupTimeout  = 20 * time.Second
	disableTaint    = true
	operatingSystem = "Linux"
	numberOfWorkers = 50
	resync          time.Duration
	qps             = float32(50)
	burst           = 100

	// Set the tls config to use for the http server
	certPath       = os.Getenv("APISERVER_CERT_LOCATION")
	keyPath        = os.Getenv("APISERVER_KEY_LOCATION")
	clientCACert   string
	clientNoVerify bool

	webhookAuth                  bool
	webhookAuthnCacheTTL         time.Duration
	webhookAuthzUnauthedCacheTTL time.Duration
	webhookAuthzAuthedCacheTTL   time.Duration
	nodeName                     = "vk-kube-mock"
	listenPort                   = 10250

	statusUpdatesInterval = 5 * time.Second
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	binaryName := filepath.Base(os.Args[0])
	desc := binaryName + " implements a node on a Kubernetes cluster to mock the running of pods"

	if kubeConfigPath == "" {
		home, _ := homedir.Dir()
		if home != "" {
			kubeConfigPath = filepath.Join(home, ".kube", "config")
		}
	}

	// cfg.Client
	// k8sClient, err := nodeutil.ClientsetFromEnv(kubeConfigPath)
	k8sClient, err := getK8sClient(kubeConfigPath)
	if err != nil {
		log.G(ctx).Fatal(err)
	}
	withClient := func(cfg *nodeutil.NodeConfig) error {
		return nodeutil.WithClient(k8sClient)(cfg)
	}

	// cfg.NodeSpec.Spec.Taints
	withTaint := func(cfg *nodeutil.NodeConfig) error {
		if disableTaint {
			return nil
		}

		taint := v1.Taint{
			Key:   taintKey,
			Value: taintValue,
		}
		switch taintEffect {
		case "NoSchedule":
			taint.Effect = v1.TaintEffectNoSchedule
		case "NoExecute":
			taint.Effect = v1.TaintEffectNoExecute
		case "PreferNoSchedule":
			taint.Effect = v1.TaintEffectPreferNoSchedule
		default:
			return errdefs.InvalidInputf("taint effect %q is not supported", taintEffect)
		}
		cfg.NodeSpec.Spec.Taints = append(cfg.NodeSpec.Spec.Taints, taint)
		return nil
	}

	// cfg.NodeSpec.Status.NodeInfo.KubeletVersion
	withVersion := func(cfg *nodeutil.NodeConfig) error {
		cfg.NodeSpec.Status.NodeInfo.KubeletVersion = strings.Join([]string{k8sVersion, "vk-kube-mock", buildVersion}, "-")
		return nil
	}

	// cfg.Handler, kubelet's routers
	configureRoutes := func(cfg *nodeutil.NodeConfig) error {
		mux := http.NewServeMux()
		cfg.Handler = mux
		return nodeutil.AttachProviderRoutes(mux)(cfg)
	}

	// cfg.TLSConfig
	// Set the tls config to use for the http server
	withCA := func(cfg *tls.Config) error {
		if clientCACert == "" {
			return nil
		}
		if err := nodeutil.WithCAFromPath(clientCACert)(cfg); err != nil {
			return fmt.Errorf("error getting CA from path: %w", err)
		}
		if clientNoVerify {
			cfg.ClientAuth = tls.NoClientCert
		}
		return nil
	}
	withTLSConfig := func(cfg *nodeutil.NodeConfig) error {
		if certPath == "" {
			return nil
		}
		if clientCACert == "" {
			return nil
		}
		nodeutil.WithTLSConfig(nodeutil.WithKeyPairFromPath(certPath, keyPath), withCA)
		return nil
	}

	// todo
	withWebhookAuth := func(cfg *nodeutil.NodeConfig) error {
		if !webhookAuth {
			cfg.Handler = api.InstrumentHandler(nodeutil.WithAuth(nodeutil.NoAuth(), cfg.Handler))
			return nil
		}

		auth, err := nodeutil.WebhookAuth(cfg.Client, nodeName,
			func(cfg *nodeutil.WebhookAuthConfig) error {
				var err error

				cfg.AuthzConfig.WebhookRetryBackoff = options.DefaultAuthWebhookRetryBackoff()

				if webhookAuthnCacheTTL > 0 {
					cfg.AuthnConfig.CacheTTL = webhookAuthnCacheTTL
				}
				if webhookAuthzAuthedCacheTTL > 0 {
					cfg.AuthzConfig.AllowCacheTTL = webhookAuthzAuthedCacheTTL
				}
				if webhookAuthzUnauthedCacheTTL > 0 {
					cfg.AuthzConfig.AllowCacheTTL = webhookAuthzUnauthedCacheTTL
				}
				if clientCACert != "" {
					ca, err := dynamiccertificates.NewDynamicCAContentFromFile("client-ca", clientCACert)
					if err != nil {
						return err
					}
					cfg.AuthnConfig.ClientCertificateCAContentProvider = ca
					go ca.Run(ctx, 1)
				}
				return err
			})

		if err != nil {
			return err
		}
		cfg.TLSConfig.ClientAuth = tls.RequestClientCert
		cfg.Handler = api.InstrumentHandler(nodeutil.WithAuth(auth, cfg.Handler))
		return nil
	}

	run := func(ctx context.Context) error {
		node, err := nodeutil.NewNode(
			nodeName,
			func(cfg nodeutil.ProviderConfig) (nodeutil.Provider, vknode.NodeProvider, error) {
				if port := os.Getenv("KUBELET_PORT"); port != "" {
					var err error
					listenPort, err = strconv.Atoi(port)
					if err != nil {
						return nil, nil, err
					}
				}
				p, err := kubemockprovider.NewMockProvider(
					ctx,
					kubemockprovider.MockConfig{
						CPU:    cpu,
						Memory: memory,
						Pods:   podCapacity,
					},
					cfg,
					nodeName,
					operatingSystem,
					os.Getenv("VKUBELET_POD_IP"),
					int32(listenPort),
					k8sClient,
					statusUpdatesInterval,
				)
				if err != nil {
					return nil, nil, err
				}
				p.ConfigureNode(ctx, cfg.Node)
				return p, nil, err
			},
			withClient,
			withTaint,
			withVersion,
			withTLSConfig,
			withWebhookAuth,
			configureRoutes,
			// other
			func(cfg *nodeutil.NodeConfig) error {
				cfg.InformerResyncPeriod = resync
				cfg.NumWorkers = numberOfWorkers
				cfg.HTTPListenAddr = fmt.Sprintf(":%d", listenPort)
				return nil
			},
		)
		if err != nil {
			return err
		}

		go func() error {
			err = node.Run(ctx)
			if err != nil {
				return fmt.Errorf("error running the node: %w", err)
			}
			return nil
		}()

		if err := node.WaitReady(ctx, startupTimeout); err != nil {
			return fmt.Errorf("error waiting for node to be ready: %w", err)
		}

		<-node.Done()
		return node.Err()
	}

	cmd := &cobra.Command{
		Use:   binaryName,
		Short: desc,
		Long:  desc,
		Run: func(cmd *cobra.Command, args []string) {
			logger := logrus.StandardLogger()
			lvl, err := logrus.ParseLevel(logLevel)
			if err != nil {
				logrus.WithError(err).Fatal("Error parsing log level")
			}
			logger.SetLevel(lvl)

			ctx := log.WithLogger(cmd.Context(), logruslogger.FromLogrus(logrus.NewEntry(logger)))

			if err := run(ctx); err != nil {
				if !errors.Is(err, context.Canceled) {
					log.G(ctx).Fatal(err)
				}
				log.G(ctx).Debug(err)
			}
		},
	}
	flags := cmd.Flags()

	klogFlags := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(klogFlags)
	klogFlags.VisitAll(func(f *flag.Flag) {
		f.Name = "klog." + f.Name
		flags.AddGoFlag(f)
	})

	flags.StringVar(&nodeName, "nodename", nodeName, "kubernetes node name")
	flags.StringVar(&cpu, "cpu", cpu, "kubernetes node cpu")
	flags.StringVar(&memory, "memory", memory, "kubernetes node memory")
	flags.StringVar(&podCapacity, "pods", podCapacity, "kubernetes node podCapacity")
	flags.StringVar(&clusterDomain, "cluster-domain", clusterDomain, "kubernetes cluster-domain")
	flags.DurationVar(&startupTimeout, "startup-timeout", startupTimeout, "How long to wait for the virtual-kubelet to start")
	flags.BoolVar(&disableTaint, "disable-taint", disableTaint, "disable the node taint")
	flags.StringVar(&operatingSystem, "os", operatingSystem, "Operating System (Linux/Windows)")
	flags.StringVar(&logLevel, "log-level", logLevel, "log level.")
	flags.IntVar(&numberOfWorkers, "pod-sync-workers", numberOfWorkers, `set the number of pod synchronization workers`)
	flags.DurationVar(&resync, "full-resync-period", resync, "how often to perform a full resync of pods between kubernetes and the provider")
	flags.Float32Var(&qps, "kube-api-qps", qps, `QPS to use while talking with kubernetes apiserver. The number must be >= 0. If 0 will use DefaultQPS: 50.`)
	flags.IntVar(&burst, "kube-api-burst", burst, `Burst to use while talking with kubernetes apiserver. The number must be >= 0. If 0 will use DefaultBurst: 100.`)

	flags.StringVar(&clientCACert, "client-verify-ca", os.Getenv("APISERVER_CA_CERT_LOCATION"), "CA cert to use to verify client requests")
	flags.BoolVar(&clientNoVerify, "no-verify-clients", clientNoVerify, "Do not require client certificate validation")
	flags.BoolVar(&webhookAuth, "authentication-token-webhook", webhookAuth, ""+
		"Use the TokenReview API to determine authentication for bearer tokens.")
	flags.DurationVar(&webhookAuthnCacheTTL, "authentication-token-webhook-cache-ttl", webhookAuthnCacheTTL,
		"The duration to cache responses from the webhook token authenticator.")
	flags.DurationVar(&webhookAuthzAuthedCacheTTL, "authorization-webhook-cache-authorized-ttl", webhookAuthzAuthedCacheTTL,
		"The duration to cache 'authorized' responses from the webhook authorizer.")
	flags.DurationVar(&webhookAuthzUnauthedCacheTTL, "authorization-webhook-cache-unauthorized-ttl", webhookAuthzUnauthedCacheTTL,
		"The duration to cache 'unauthorized' responses from the webhook authorizer.")

	flags.DurationVar(&statusUpdatesInterval, "status-updates-interval", statusUpdatesInterval,
		"status updates interval. Default is 5s.")

	// deprecated flags
	flags.StringVar(&taintKey, "taint", taintKey, "Set node taint key")
	flags.MarkDeprecated("taint", "Taint key should now be configured using the VKUBELET_TAINT_KEY environment variable")

	if err := cmd.ExecuteContext(ctx); err != nil {
		if !errors.Is(err, context.Canceled) {
			logrus.WithError(err).Fatal("Error running command")
		}
	}
}

func envOrDefault(key string, defaultValue string) string {
	v, set := os.LookupEnv(key)
	if set {
		return v
	}
	return defaultValue
}

func getK8sClient(kubeConfigPath string) (*kubernetes.Clientset, error) {

	// use the current context in kubeconfig
	var config *restclient.Config
	var err error

	if kubeConfigPath == "" {
		// creates the in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	} else {
		// use the current context in kubeconfig
		config, err = clientcmd.BuildConfigFromFlags("", kubeConfigPath)
		if err != nil {
			return nil, err
		}
	}

	config.QPS = qps
	config.Burst = burst

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}
