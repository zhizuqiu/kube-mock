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

	// todo delete node

	return el, nil
}
