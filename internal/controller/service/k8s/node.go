package k8s

import (
	"context"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Node interface {
	GetNodeOnly(name string) (*v1.Node, error)
}

type NodeService struct {
	KubeClient client.Client
	Log        logr.Logger
}

func NewNodeService(kubeClient client.Client, log logr.Logger) *NodeService {
	log = log.WithValues("service", "k8s.NodeService")
	return &NodeService{
		KubeClient: kubeClient,
		Log:        log,
	}
}

func (n NodeService) GetNodeOnly(name string) (*v1.Node, error) {
	var node = &v1.Node{}
	err := n.KubeClient.Get(context.Background(),
		types.NamespacedName{
			Name: name,
		},
		node,
	)
	return node, err
}
