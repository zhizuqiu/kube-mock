package cmd

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"github.com/zhizuqiu/kube-mock/internal/util"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apiserver/pkg/server/dynamiccertificates"
	"k8s.io/apiserver/pkg/server/options"
	"k8s.io/klog/v2"
	"net/http"
	"os"
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

	labelsStr   string
	taintsStr   string
	labels      map[string]string
	taints      []v1.Taint
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

	descNode = "Implements a node on a Kubernetes cluster to mock the running of pods"
)

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: descNode,
	Long:  descNode,
	Run: func(cmd *cobra.Command, args []string) {
		logger := logrus.StandardLogger()
		lvl, err := logrus.ParseLevel(logLevel)
		if err != nil {
			logrus.WithError(err).Fatal("Error parsing log level")
		}
		logger.SetLevel(lvl)

		ctx := log.WithLogger(cmd.Context(), logruslogger.FromLogrus(logrus.NewEntry(logger)))

		if err := runNode(ctx); err != nil {
			if !errors.Is(err, context.Canceled) {
				log.G(ctx).Fatal(err)
			}
			log.G(ctx).Debug(err)
		}
	},
}

func init() {
	// klog.v
	flags := nodeCmd.Flags()
	klogFlags := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(klogFlags)
	klogFlags.VisitAll(func(f *flag.Flag) {
		f.Name = "klog." + f.Name
		flags.AddGoFlag(f)
	})

	nodeCmd.Flags().StringVar(&nodeName, "nodename", nodeName, "kubernetes node name")
	nodeCmd.Flags().StringVar(&labelsStr, "labels", nodeName, "kubernetes node labels (key1=value1,key2=value2)")
	nodeCmd.Flags().StringVar(&taintsStr, "taints", nodeName, "kubernetes node taints (json)")
	nodeCmd.Flags().StringVar(&cpu, "cpu", cpu, "kubernetes node cpu")
	nodeCmd.Flags().StringVar(&memory, "memory", memory, "kubernetes node memory")
	nodeCmd.Flags().StringVar(&podCapacity, "pods", podCapacity, "kubernetes node podCapacity")
	nodeCmd.Flags().StringVar(&clusterDomain, "cluster-domain", clusterDomain, "kubernetes cluster-domain")
	nodeCmd.Flags().DurationVar(&startupTimeout, "startup-timeout", startupTimeout, "How long to wait for the virtual-kubelet to start")
	nodeCmd.Flags().BoolVar(&disableTaint, "disable-taint", disableTaint, "disable the node taint")
	nodeCmd.Flags().StringVar(&operatingSystem, "os", operatingSystem, "Operating System (Linux/Windows)")
	nodeCmd.Flags().StringVar(&logLevel, "log-level", logLevel, "log level.")
	nodeCmd.Flags().IntVar(&numberOfWorkers, "pod-sync-workers", numberOfWorkers, `set the number of pod synchronization workers`)
	nodeCmd.Flags().DurationVar(&resync, "full-resync-period", resync, "how often to perform a full resync of pods between kubernetes and the provider")

	nodeCmd.Flags().StringVar(&clientCACert, "client-verify-ca", os.Getenv("APISERVER_CA_CERT_LOCATION"), "CA cert to use to verify client requests")
	nodeCmd.Flags().BoolVar(&clientNoVerify, "no-verify-clients", clientNoVerify, "Do not require client certificate validation")
	nodeCmd.Flags().BoolVar(&webhookAuth, "authentication-token-webhook", webhookAuth, ""+
		"Use the TokenReview API to determine authentication for bearer tokens.")
	nodeCmd.Flags().DurationVar(&webhookAuthnCacheTTL, "authentication-token-webhook-cache-ttl", webhookAuthnCacheTTL,
		"The duration to cache responses from the webhook token authenticator.")
	nodeCmd.Flags().DurationVar(&webhookAuthzAuthedCacheTTL, "authorization-webhook-cache-authorized-ttl", webhookAuthzAuthedCacheTTL,
		"The duration to cache 'authorized' responses from the webhook authorizer.")
	nodeCmd.Flags().DurationVar(&webhookAuthzUnauthedCacheTTL, "authorization-webhook-cache-unauthorized-ttl", webhookAuthzUnauthedCacheTTL,
		"The duration to cache 'unauthorized' responses from the webhook authorizer.")

	nodeCmd.Flags().DurationVar(&statusUpdatesInterval, "status-updates-interval", statusUpdatesInterval,
		"status updates interval. Default is 5s.")

	// deprecated flags
	nodeCmd.Flags().StringVar(&taintKey, "taint", taintKey, "Set node taint key")
	nodeCmd.Flags().MarkDeprecated("taint", "Taint key should now be configured using the VKUBELET_TAINT_KEY environment variable")
	rootCmd.AddCommand(nodeCmd)
}

func runNode(ctx context.Context) error {
	if kubeConfigPath == "" {
		home, _ := homedir.Dir()
		if home != "" {
			kubeConfigPath = filepath.Join(home, ".kube", "config")
		}
	}

	labels = util.LabelsStrToMap(labelsStr)

	_ = json.Unmarshal([]byte(taintsStr), &taints)

	// cfg.Client
	k8sClient, err := util.GetK8sClient(kubeConfigPath, qps, burst)
	if err != nil {
		log.G(ctx).Fatal(err)
	}
	withClient := func(cfg *nodeutil.NodeConfig) error {
		return nodeutil.WithClient(k8sClient)(cfg)
	}

	// cfg.NodeSpec.Labels
	withLabels := func(cfg *nodeutil.NodeConfig) error {
		for key, value := range labels {
			cfg.NodeSpec.Labels[key] = value
		}
		return nil
	}

	// cfg.NodeSpec.Spec.Taints
	withTaint := func(cfg *nodeutil.NodeConfig) error {
		if taintsStr != "" {
			cfg.NodeSpec.Spec.Taints = taints
		} else {
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
		}
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
		withLabels,
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

func envOrDefault(key string, defaultValue string) string {
	v, set := os.LookupEnv(key)
	if set {
		return v
	}
	return defaultValue
}
