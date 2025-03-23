/*
Copyright 2025.

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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	akuityiov1 "akuity.io/namespaceclass/api/v1"
)

var (
	NamespaceClassOwnerKey = ".metadata.controller"
	ApiGVStr               = akuityiov1.GroupVersion.String()
)

// NamespaceClassLabel is the label used to identify the namespace class that namespace resource refers to.
const NamespaceClassLabel = "namespaceclass.akuity.io/name"

// NamespaceClassReconciler reconciles a NamespaceClass object
type NamespaceClassReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=akuity.io,resources=namespaceclasses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=akuity.io,resources=namespaceclasses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=akuity.io,resources=namespaceclasses/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NamespaceClass object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *NamespaceClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if req.Namespace != "" {
		return r.HandleForNamespaceChange(ctx, req)
	}
	return r.HandleForNamespaceClassChange(ctx, req)
}

func (r *NamespaceClassReconciler) HandleForNamespaceClassChange(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("action", "HandleForNamespaceClassChange")

	var namespaceClass akuityiov1.NamespaceClass
	if err := r.Client.Get(ctx, types.NamespacedName{Name: req.Name}, &namespaceClass); err != nil {
		logger.Error(err, "unable to fetch NamespaceClass")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var namespaceList corev1.NamespaceList
	if err := r.Client.List(ctx, &namespaceList, client.MatchingLabels{NamespaceClassLabel: req.Name}); err != nil {
		logger.Error(err, "unable to list Namespaces")
		return ctrl.Result{}, err
	}

	var networkingList akuityiov1.NetworkingList
	if err := r.Client.List(ctx, &networkingList, client.MatchingFields{NamespaceClassOwnerKey: req.Name}); err != nil {
		logger.Error(err, "unable to list Networking")
		return ctrl.Result{}, err
	}

	networkingMap := make(map[string]*akuityiov1.Networking, len(networkingList.Items))
	for _, networking := range networkingList.Items {
		networkingMap[networking.Namespace] = &networking
	}

	// update networking resources with the latest networking spec
	for _, networking := range networkingList.Items {
		networking.Spec = *namespaceClass.Spec.Networking.DeepCopy()
		if err := r.Client.Update(ctx, &networking); err != nil {
			logger.Error(err, "unable to update Networking")
		}
	}

	// create networking resources for the namespaces that do not have networking resources
	for _, namespace := range namespaceList.Items {
		if _, ok := networkingMap[namespace.Name]; !ok {
			// for this namespace, the networking resource does not exist
			// so create the networking resource
			if err := r.createNetworking(ctx, &namespace, &namespaceClass); err != nil {
				logger.Error(err, "unable to create Networking")
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *NamespaceClassReconciler) createNetworking(ctx context.Context, namespace *corev1.Namespace, namespaceClass *akuityiov1.NamespaceClass) error {
	networking := akuityiov1.Networking{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespaceClass.Name,
			Namespace: namespace.Name,
		},
		Spec: *namespaceClass.Spec.Networking.DeepCopy(),
	}
	if err := ctrl.SetControllerReference(namespaceClass, &networking, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}
	if err := r.Client.Create(ctx, &networking); err != nil {
		return fmt.Errorf("failed to create Networking: %w", err)
	}
	return nil
}

func (r *NamespaceClassReconciler) HandleForNamespaceChange(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("action", "HandleForNamespaceChange")

	var namespace corev1.Namespace
	if err := r.Client.Get(ctx, client.ObjectKey{Name: req.Namespace}, &namespace); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if namespace.Status.Phase == corev1.NamespaceTerminating {
		// Namespace is terminating, do nothing
		logger.Info("namespace is terminating")
		return ctrl.Result{}, nil
	}

	namespaceClassName, ok := namespace.Labels[NamespaceClassLabel]
	if !ok {
		// Namespace has removed the label, delete the networking resource if exists
		logger.Info("namespace has removed the label")

		if err := r.deleteNetworking(ctx, req.Name, req.Namespace); err != nil {
			logger.Error(err, "unable to delete Networking")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	if namespaceClassName != req.Name {
		logger.Info("namespace has changed the label")
		// Namespace has changed the label, delete previous networking resource if exists, create new networking resource

		if err := r.deleteNetworking(ctx, req.Name, req.Namespace); err != nil {
			// for this scenario, we simply try to delete the networking resource
			// if it fails, we log the error and continue
			logger.Error(err, "unable to delete Networking")
		}

		var namespaceClass akuityiov1.NamespaceClass
		if err := r.Client.Get(ctx, client.ObjectKey{Name: namespaceClassName}, &namespaceClass); err != nil {
			logger.Error(err, "unable to fetch NamespaceClass")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		if err := r.createNetworking(ctx, &namespace, &namespaceClass); err != nil {
			logger.Error(err, "unable to create Networking")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	{
		// namespaceClassName == req.Name
		// Namespace has not changed the label, create networking resource if not exists
		// This is the case where the namespace has the label and it matches the namespace class name

		logger.Info("namespace matches")

		var networking akuityiov1.Networking
		err := r.Client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, &networking)
		if err == nil {
			return ctrl.Result{}, nil
		}

		if apierrors.IsNotFound(err) {
			// networking resource does not exist,
			// means the namespace has the label but networking resource is not created yet

			var namespaceClass akuityiov1.NamespaceClass
			if err := r.Client.Get(ctx, client.ObjectKey{Name: req.Name}, &namespaceClass); err != nil {
				logger.Error(err, "unable to fetch NamespaceClass")
				return ctrl.Result{}, client.IgnoreNotFound(err)
			}

			if err := r.createNetworking(ctx, &namespace, &namespaceClass); err != nil {
				logger.Error(err, "unable to create Networking")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}
}

func (r *NamespaceClassReconciler) deleteNetworking(ctx context.Context, name string, namespace string) error {
	var networking akuityiov1.Networking
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &networking); err != nil {
		return fmt.Errorf("failed to fetch Networking: %w", err)
	}
	if err := r.Client.Delete(ctx, &networking, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
		return fmt.Errorf("failed to delete Networking: %w", err)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceClassReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &akuityiov1.Networking{}, NamespaceClassOwnerKey, func(rawObj client.Object) []string {
		// grab the Networking object, extract the owner...
		networking := rawObj.(*akuityiov1.Networking)
		owner := metav1.GetControllerOf(networking)
		if owner == nil {
			return nil
		}
		// ...make sure it's a NamespaceClass...
		if owner.APIVersion != ApiGVStr || owner.Kind != "NamespaceClass" {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&akuityiov1.NamespaceClass{}).
		Named("namespaceclass").
		Owns(&akuityiov1.Networking{}).
		Watches(&corev1.Namespace{}, handler.EnqueueRequestsFromMapFunc(r.watchNamespaceResource), builder.WithPredicates(NamespacePredicate)).
		Complete(r)
}

// watchNamespaceResource watches the namespace resource with the given namespace class label
// and returns the reconcile requests for the namespace class resource.
func (r *NamespaceClassReconciler) watchNamespaceResource(_ context.Context, a client.Object) []reconcile.Request {
	namespace, isNamespaceObject := a.(*corev1.Namespace)
	if !isNamespaceObject {
		return nil
	}
	namespaceClass, ok := namespace.Labels[NamespaceClassLabel]
	if !ok {
		return nil
	}
	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Name:      namespaceClass,
			Namespace: namespace.Name,
		},
	}}
}

// NamespacePredicate is the predicate used to filter the namespace resource with the given namespace class label.
var NamespacePredicate = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		namespace, isNamespaceObject := e.Object.(*corev1.Namespace)
		if !isNamespaceObject {
			return false
		}
		_, ok := namespace.Labels[NamespaceClassLabel]
		return ok
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		oldNamespace, _ := e.ObjectOld.(*corev1.Namespace)
		newNamespace, _ := e.ObjectNew.(*corev1.Namespace)

		_, oldOk := oldNamespace.Labels[NamespaceClassLabel]
		_, newOk := newNamespace.Labels[NamespaceClassLabel]

		return oldOk || newOk
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}
