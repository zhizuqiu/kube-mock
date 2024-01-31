package k8s

import (
	"context"
	"errors"
	"github.com/go-logr/logr"
	"github.com/zhizuqiu/kube-mock/api/v1alpha1"
	apl "k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	statusUpdateRetries = 3
)

type Node interface {
	GetNodeOnly(ctx context.Context, req ctrl.Request) (*v1alpha1.Node, error)
	GetNode(ctx context.Context, req ctrl.Request) (*v1alpha1.Node, error)
	UpdateNodeStatus(ctx context.Context, req ctrl.Request, node *v1alpha1.Node) error
	GetNodeList(ctx context.Context, req ctrl.Request, labels map[string]string) (*v1alpha1.NodeList, error)
}

type NodeService struct {
	KubeClient client.Client
	Log        logr.Logger
}

func NewNodeService(kubeClient client.Client, log logr.Logger) *NodeService {
	log = log.WithValues("service", "k8s.CRDService")
	return &NodeService{
		KubeClient: kubeClient,
		Log:        log,
	}
}

func (r *NodeService) GetNodeOnly(ctx context.Context, req ctrl.Request) (*v1alpha1.Node, error) {

	r.Log.Info("start fetch Node yaml...", "req", req)

	// Load the Node by name
	node := &v1alpha1.Node{}
	err := r.KubeClient.Get(ctx, req.NamespacedName, node)

	return node, err
}

func (r *NodeService) GetNode(ctx context.Context, req ctrl.Request) (*v1alpha1.Node, error) {

	node, err := r.GetNodeOnly(ctx, req)
	if err != nil {
		r.Log.Error(err, "unable to fetch node", "req", req)
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		// return nil, client.IgnoreNotFound(err)
		return nil, err
	}
	if node.GetObjectMeta().GetDeletionTimestamp() != nil {
		return nil, errors.New("this crd has been deleted")
	}

	return node, nil
}

func (r *NodeService) UpdateNodeStatus(ctx context.Context, req ctrl.Request, node *v1alpha1.Node) error {

	var err error
	for i := 0; i <= statusUpdateRetries; i = i + 1 {
		newNode := &v1alpha1.Node{}
		err = r.KubeClient.Get(ctx, req.NamespacedName, newNode)
		if err != nil {
			break
		}
		newNode.Status = node.Status
		err = r.KubeClient.Status().Update(ctx, newNode)
		if err == nil {
			break
		} else {
			r.Log.Error(err, "update")
		}
	}
	return err
}

func (r *NodeService) GetNodeList(ctx context.Context, req ctrl.Request, labels map[string]string) (*v1alpha1.NodeList, error) {
	nodeList := &v1alpha1.NodeList{}
	err := r.KubeClient.List(ctx, nodeList, &client.ListOptions{
		Namespace:     req.NamespacedName.Namespace,
		LabelSelector: apl.SelectorFromSet(labels),
	})
	if err != nil {
		r.Log.Error(err, "unable to list node", "req", req)
		return nil, err
	}

	return nodeList, err
}
