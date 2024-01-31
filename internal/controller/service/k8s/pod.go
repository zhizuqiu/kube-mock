package k8s

import (
	"context"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	apl "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Pod interface {
	ListPods(namespace string, labels map[string]string) (*v1.PodList, error)
}

type PodService struct {
	KubeClient client.Client
	Log        logr.Logger
}

func NewPodService(kubeClient client.Client, log logr.Logger) *PodService {
	log = log.WithValues("service", "k8s.PodService")
	return &PodService{
		KubeClient: kubeClient,
		Log:        log,
	}
}

func (p PodService) ListPods(namespace string, labels map[string]string) (*v1.PodList, error) {
	var podList = &v1.PodList{}
	if err := p.KubeClient.List(context.Background(),
		podList,
		&client.ListOptions{
			Namespace:     namespace,
			LabelSelector: apl.SelectorFromSet(labels),
		},
	); err != nil {
		return nil, err
	}
	return podList, nil
}
