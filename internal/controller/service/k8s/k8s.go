package k8s

import (
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Service is the K8s service entrypoint.
type NodeServices interface {
	client.Client
	Node
	Pod
}

type nodeServices struct {
	client.Client
	Node
	Pod
}

func NewNodeServices(kubeClient client.Client, log logr.Logger) NodeServices {
	return &nodeServices{
		Client: kubeClient,
		Node:   NewNodeService(kubeClient, log),
		Pod:    NewPodService(kubeClient, log),
	}
}
