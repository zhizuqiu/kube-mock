package controller

import (
	"github.com/go-logr/logr"
	"github.com/zhizuqiu/kube-mock/api/v1alpha1"
	"github.com/zhizuqiu/kube-mock/internal/controller/service/ensure"
	"github.com/zhizuqiu/kube-mock/internal/controller/service/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type NodeHandler struct {
	Ensurer     ensure.NodeEnsure
	K8sServices k8s.NodeServices
	Log         logr.Logger
}

func NewNodeHandler(ensurer ensure.NodeEnsure, k8sServices k8s.NodeServices, log logr.Logger) *NodeHandler {
	return &NodeHandler{
		Ensurer:     ensurer,
		K8sServices: k8sServices,
		Log:         log,
	}
}

func (r *NodeHandler) createOwnerReferences(node *v1alpha1.Node) []metav1.OwnerReference {
	rfvk := schema.GroupVersionKind{
		Group:   v1alpha1.GroupVersion.Group,
		Version: v1alpha1.GroupVersion.Version,
		Kind:    "Node",
	}
	return []metav1.OwnerReference{
		*metav1.NewControllerRef(node, rfvk),
	}
}
