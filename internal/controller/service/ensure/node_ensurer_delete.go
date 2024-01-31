package ensure

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/zhizuqiu/kube-mock/internal/controller/service/element"
	"github.com/zhizuqiu/kube-mock/internal/controller/service/k8s"
	apperrors "k8s.io/apimachinery/pkg/api/errors"
)

type NodeDeleteEnsure interface {
	DeleteEnsureNode(el element.NodeElement) (element.NodeElement, error)
}

type NodeDeleteEnsurer struct {
	K8SService k8s.NodeServices
	Log        logr.Logger
}

func NewNodeDeleteEnsurer(k8sService k8s.NodeServices, log logr.Logger) *NodeDeleteEnsurer {
	log = log.WithValues("ensure", "NodeDeleteEnsurer")
	return &NodeDeleteEnsurer{
		K8SService: k8sService,
		Log:        log,
	}
}

func (e *NodeDeleteEnsurer) DeleteEnsureNode(el element.NodeElement) (element.NodeElement, error) {
	node, err := e.K8SService.GetNodeOnly(el.Node.Name)
	if err != nil {
		if apperrors.IsNotFound(err) {
			e.Log.Info("Node resource not found. Ignoring since object must be deleted.")
			return el, nil
		}
		e.Log.Error(err, "Failed to get Node.")
		return el, err
	}
	if node.DeletionTimestamp == nil {
		err = e.K8SService.Delete(context.Background(), node)
		if err != nil {
			return el, err
		}
	}
	return el, nil
}
