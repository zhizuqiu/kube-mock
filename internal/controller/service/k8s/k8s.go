package k8s

import (
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Service is the K8s service entrypoint.
type NodeServices interface {
	client.Client
	NodeCrd
	Pod
	Node
}

type nodeServices struct {
	client.Client
	NodeCrd
	Pod
	Node
}

func NewNodeServices(kubeClient client.Client, log logr.Logger) NodeServices {
	return &nodeServices{
		Client:  kubeClient,
		NodeCrd: NewNodeCrdService(kubeClient, log),
		Pod:     NewPodService(kubeClient, log),
		Node:    NewNodeService(kubeClient, log),
	}
}
