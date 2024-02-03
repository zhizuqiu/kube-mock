/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"github.com/go-logr/logr"
	mockv1alpha1 "github.com/zhizuqiu/kube-mock/api/v1alpha1"
	"github.com/zhizuqiu/kube-mock/internal/controller/service/element"
	"github.com/zhizuqiu/kube-mock/internal/util"
	v1 "k8s.io/api/core/v1"
	apperrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"
)

var (
	ErrorRequeueAfter = 10 * time.Second
)

// NodeReconciler reconciles a Node object
type NodeReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Log         logr.Logger
	NodeHandler *NodeHandler
}

//+kubebuilder:rbac:groups=mock.zhizuqiu.cn,resources=nodes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mock.zhizuqiu.cn,resources=nodes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mock.zhizuqiu.cn,resources=nodes/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=nodes/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Node object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	node, err := r.NodeHandler.K8sServices.GetNodeCrdOnly(ctx, req)
	if err != nil {
		if apperrors.IsNotFound(err) {
			r.Log.Info("NodeCrd resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		r.Log.Error(err, "Failed to get NodeCrd.")
		return ctrl.Result{}, err
	}

	// Create owner refs so the objects manager by this handler have ownership to the
	// received RF.
	oRefs := r.NodeHandler.createOwnerReferences(node)

	el := element.NodeElement{
		Ctx:       ctx,
		Reconcile: false,
		Req:       req,
		Node:      node,
		OwnerRefs: oRefs,
	}

	isNodeMarkedToBeDeleted := el.Node.GetDeletionTimestamp() != nil
	if isNodeMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(el.Node, util.NodeFinalizer) {
			// Run finalization logic for NodeFinalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			if err := r.finalizeRedis(r.Log, el); err != nil {
				r.Log.Error(err, "finalizeRedis error!", "req", req)
				return ctrl.Result{RequeueAfter: ErrorRequeueAfter}, nil
			}

			// Remove RedisFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(el.Node, util.NodeFinalizer)
			err := r.Update(context.Background(), el.Node)
			if err != nil {
				r.Log.Error(err, "Remove RedisFinalizer, Update CR error!", el.Node)
				return ctrl.Result{RequeueAfter: ErrorRequeueAfter}, nil
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer for this CR
	if !isNodeMarkedToBeDeleted && !controllerutil.ContainsFinalizer(el.Node, util.NodeFinalizer) {
		controllerutil.AddFinalizer(el.Node, util.NodeFinalizer)
		err = r.Update(context.Background(), el.Node)
		if err != nil {
			r.Log.Error(err, "Add finalizer for this CR, Update CR error!", el.Node)
			return ctrl.Result{RequeueAfter: ErrorRequeueAfter}, nil
		}
	}

	el, err = r.Ensure(el)
	if err != nil {
		r.Log.Error(err, "Ensure error!", "req", req)
		return ctrl.Result{}, err
	}
	if el.Reconcile {
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *NodeReconciler) finalizeRedis(reqLogger logr.Logger, el element.NodeElement) error {
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	el, err := r.DeleteEnsure(el)
	if err != nil {
		return err
	}

	reqLogger.Info("Successfully finalized node")
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mockv1alpha1.Node{}).
		Owns(&v1.Pod{}).
		Complete(r)
}
