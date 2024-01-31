package element

import (
	"context"
	"github.com/zhizuqiu/kube-mock/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

type NodeElement struct {
	Ctx context.Context
	// if true wait next reconcile
	Reconcile bool
	Req       ctrl.Request
	Node      *v1alpha1.Node
	OwnerRefs []metav1.OwnerReference
}
