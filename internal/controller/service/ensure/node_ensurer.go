package ensure

import (
	"github.com/go-logr/logr"
	"github.com/zhizuqiu/kube-mock/internal/controller/service/element"
	"github.com/zhizuqiu/kube-mock/internal/controller/service/k8s"
	"github.com/zhizuqiu/kube-mock/internal/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

var (
	defaultNeed = 1
)

type NodeEnsure interface {
	EnsureNodePod(el element.NodeElement) (element.NodeElement, error)
}

type NodeEnsurer struct {
	K8SService k8s.NodeServices
	Log        logr.Logger
}

func NewNodeEnsurer(k8sService k8s.NodeServices, log logr.Logger) *NodeEnsurer {
	log = log.WithValues("ensure", "NodeEnsurer")
	return &NodeEnsurer{
		K8SService: k8sService,
		Log:        log,
	}
}

func (r *NodeEnsurer) EnsureNodePod(el element.NodeElement) (element.NodeElement, error) {
	labels := util.GetNodeLabels(el.Node)

	podList, err := r.K8SService.ListPods(el.Node.Namespace, labels)
	if err != nil {
		return el, err
	}

	// Ignore inactive pods.
	filteredPods := util.FilterActivePods(r.Log, podList.Items)

	// todo claimPods

	need := defaultNeed
	if el.Node.Spec.Suspend {
		need = 0
	}

	// create or delete pod
	el, err = r.diffPods(need, el, filteredPods)
	if err != nil {
		return el, err
	}
	if el.Reconcile {
		return el, nil
	}

	// sync pod
	el, err = r.syncPods(need, el, filteredPods)
	if err != nil {
		return el, err
	}
	if el.Reconcile {
		return el, nil
	}

	return el, nil
}

func (r *NodeEnsurer) diffPods(need int, el element.NodeElement, filteredPods []v1.Pod) (element.NodeElement, error) {
	diff := len(filteredPods) - need

	if diff < 0 {
		el.Reconcile = true
		r.Log.Info("Too few pod", "node", klog.KObj(el.Node), "need", need, "creating", -diff)
		return r.createPod(el, diff)
	} else if diff > 0 {
		el.Reconcile = true
		r.Log.Info("Too many pod", "node", klog.KObj(el.Node), "need", need, "deleting", diff)
		return r.deletePods(need, el, filteredPods, diff)
	}
	return el, nil
}

func (r *NodeEnsurer) createPod(el element.NodeElement, diff int) (element.NodeElement, error) {
	for i := 0; i < -diff; i++ {
		pod := util.CreateNodePodObj(el.Node, el.OwnerRefs)
		if err := r.K8SService.Create(el.Ctx, pod); err != nil {
			return el, err
		}
	}
	return el, nil
}

func (r *NodeEnsurer) deletePods(need int, el element.NodeElement, filteredPods []v1.Pod, diff int) (element.NodeElement, error) {
	for i := diff; i >= need; i-- {
		if err := r.K8SService.Delete(el.Ctx, &filteredPods[i]); err != nil {
			return el, err
		}
	}
	return el, nil
}

func (r *NodeEnsurer) syncPods(need int, el element.NodeElement, filteredPods []v1.Pod) (element.NodeElement, error) {
	for i := 0; i < need; i++ {
		pod := filteredPods[i]
		if !util.IsDesired(el.Node, pod) {
			r.Log.Info("pod config diff", "node", klog.KObj(el.Node))
			el.Reconcile = true
			if err := r.K8SService.Delete(el.Ctx, &filteredPods[i]); err != nil {
				return el, err
			}
		}
	}
	return el, nil
}
