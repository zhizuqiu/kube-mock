package controller

import (
	"github.com/zhizuqiu/kube-mock/internal/controller/service/element"
	"github.com/zhizuqiu/kube-mock/internal/controller/util"
)

func (r *NodeReconciler) Ensure(el element.NodeElement) (element.NodeElement, error) {
	// log := r.Log.WithValues("controllers", "NodeEnsure")
	err := util.NilError()

	if el, err = r.NodeHandler.Ensurer.EnsureNodePod(el); err != nil {
		return el, err
	}
	if el.Reconcile {
		return el, nil
	}

	return el, nil
}

func (r *NodeReconciler) DeleteEnsure(el element.NodeElement) (element.NodeElement, error) {
	err := util.NilError()

	if el, err = r.NodeHandler.DeleteEnsurer.DeleteEnsureNode(el); err != nil {
		return el, err
	}
	if el.Reconcile {
		return el, nil
	}

	return el, nil
}
