package cmd

import (
	"context"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	logruslogger "github.com/virtual-kubelet/virtual-kubelet/log/logrus"
	"github.com/zhizuqiu/kube-mock/internal/mock"
	"path/filepath"
)

var (
	fromKubeconfig = ""
	labelSelector  = ""
	fieldSelector  = ""
)
var mockCmd = &cobra.Command{
	Use:     "mock",
	Aliases: []string{"mock"},
	Args:    cobra.ExactArgs(1),
	Short:   "Generate Node CRs form kubernetes",
	Long: `Generate Node CRs form kubernetes

Usage:
kubemock mock --from-kubeconfig /path/to/kubeconfig.yaml /path/to/nodes.yaml
`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := logrus.StandardLogger()
		lvl, err := logrus.ParseLevel(logLevel)
		if err != nil {
			logrus.WithError(err).Fatal("Error parsing log level")
		}
		logger.SetLevel(lvl)

		ctx := log.WithLogger(cmd.Context(), logruslogger.FromLogrus(logrus.NewEntry(logger)))

		if err := runMock(ctx, args); err != nil {
			if !errors.Is(err, context.Canceled) {
				log.G(ctx).Fatal(err)
			}
			log.G(ctx).Debug(err)
		}
		log.G(ctx).Info("Successfully generated Node CRs from Kubernetes!")
	},
}

func init() {
	mockCmd.Flags().StringVarP(&fromKubeconfig, "from-kubeconfig", "f", "", "source kubernetes kubeconfig file path")
	mockCmd.Flags().StringVarP(&labelSelector, "selector", "l", "", `Selector (label query) to filter on, supports '=', '==', and '!='.(e.g. -l key1=value1,key2=value2). Matching
        objects must satisfy all of the specified label constraints.`)
	mockCmd.Flags().StringVar(&fieldSelector, "field-selector", "", `Selector (field query) to filter on, supports '=', '==', and '!='.(e.g. --field-selector
        key1=value1,key2=value2). The server only supports a limited number of field queries per type.`)

	rootCmd.AddCommand(mockCmd)
}

func runMock(ctx context.Context, args []string) error {
	if fromKubeconfig == "" {
		home, _ := homedir.Dir()
		if home != "" {
			fromKubeconfig = filepath.Join(home, ".kube", "config")
		}
	}
	path := args[0]
	if path == "" {
		path = "nodes.yaml"
	}

	return mock.Run(ctx, fromKubeconfig, qps, burst, labelSelector, fieldSelector, path)
}
