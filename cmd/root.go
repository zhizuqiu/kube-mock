package cmd

import (
	"context"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"os/signal"
)

var (
	qps   = float32(50)
	burst = 100
	desc  = "kubemock implements a node on a Kubernetes cluster to mock the running of pods"
)

var rootCmd = &cobra.Command{
	Use:   "kubemock",
	Short: desc,
	Long:  desc,
}

func Execute() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		if !errors.Is(err, context.Canceled) {
			logrus.WithError(err).Fatal("Error running command")
		}
	}
}

func init() {
	rootCmd.Flags().Float32Var(&qps, "kube-api-qps", qps, `QPS to use while talking with kubernetes apiserver. The number must be >= 0. If 0 will use DefaultQPS: 50.`)
	rootCmd.Flags().IntVar(&burst, "kube-api-burst", burst, `Burst to use while talking with kubernetes apiserver. The number must be >= 0. If 0 will use DefaultBurst: 100.`)
}
