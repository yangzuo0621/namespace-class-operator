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

	corev1 "k8s.io/api/core/v1"
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

// NamespaceClassReconciler reconciles a NamespaceClass object
type NamespaceClassReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=akuity.io,resources=namespaceclasses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=akuity.io,resources=namespaceclasses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=akuity.io,resources=namespaceclasses/finalizers,verbs=update

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
	logger := log.FromContext(ctx)

	var namespaceClass akuityiov1.NamespaceClass
	if err := r.Client.Get(ctx, types.NamespacedName{Name: req.Name}, &namespaceClass); err != nil {
		logger.Error(err, "unable to fetch NamespaceClass")
		return ctrl.Result{}, err
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

	{ // handle namespace update
		for _, networking := range networkingList.Items {
			networking.Spec = *namespaceClass.Spec.Networking.DeepCopy()
			if err := r.Client.Update(ctx, &networking); err != nil {
				logger.Error(err, "unable to update Networking")
			}
		}
	}

	{ // handle namespace creation
		for _, namespace := range namespaceList.Items {
			if _, ok := networkingMap[namespace.Name]; ok {
				continue
			}
			networking := akuityiov1.Networking{
				ObjectMeta: metav1.ObjectMeta{
					Name:      namespaceClass.Name,
					Namespace: namespace.Namespace,
				},
				Spec: *namespaceClass.Spec.Networking.DeepCopy(),
			}
			if err := ctrl.SetControllerReference(&namespaceClass, &networking, r.Scheme); err != nil {
				logger.Error(err, "unable to set controller reference")
				return ctrl.Result{}, err
			}
			if err := r.Client.Create(ctx, &networking); err != nil {
				logger.Error(err, "unable to create Networking")
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *NamespaceClassReconciler) HandleForNamespaceChange(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var namespace corev1.Namespace
	if err := r.Client.Get(ctx, client.ObjectKey{Name: req.Namespace}, &namespace); err != nil {
		logger.Error(err, "unable to fetch Namespace")
		return ctrl.Result{}, err
	}

	namespaceClassName, ok := namespace.Labels[NamespaceClassLabel]
	if !ok { // Namespace has removed the label, delete the networking resource
		var networking akuityiov1.Networking
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, &networking); err != nil {
			logger.Error(err, "unable to fetch Networking")
			return ctrl.Result{}, err
		}
		if err := r.Client.Delete(ctx, &networking, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			logger.Error(err, "unable to delete Networking")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if namespaceClassName != req.Name { // Namespace has changed the label, delete previous networking resource if exists, create new networking resource
		var networking akuityiov1.Networking
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, &networking); err != nil {
			logger.Error(err, "unable to fetch Networking")
			return ctrl.Result{}, err
		}
		if err := r.Client.Delete(ctx, &networking, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			logger.Error(err, "unable to delete Networking")
			return ctrl.Result{}, err
		}

		var namespaceClass akuityiov1.NamespaceClass
		if err := r.Client.Get(ctx, types.NamespacedName{Name: req.Name}, &namespaceClass); err != nil {
			logger.Error(err, "unable to fetch NamespaceClass")
			return ctrl.Result{}, err
		}

		networking = akuityiov1.Networking{
			ObjectMeta: metav1.ObjectMeta{
				Name:      req.Name,
				Namespace: req.Namespace,
			},
			Spec: *namespaceClass.Spec.Networking.DeepCopy(),
		}
		if err := ctrl.SetControllerReference(&namespaceClass, &networking, r.Scheme); err != nil {
			logger.Error(err, "unable to set controller reference")
			return ctrl.Result{}, err
		}
		if err := r.Client.Create(ctx, &networking); err != nil {
			logger.Error(err, "unable to create Networking")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// Namespace has not changed the label, update the networking resource if exists
	{
		var networking akuityiov1.Networking
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, &networking); err != nil {
			logger.Error(err, "unable to fetch Networking")
			return ctrl.Result{}, err
		}
		var namespaceClass akuityiov1.NamespaceClass
		if err := r.Client.Get(ctx, types.NamespacedName{Name: req.Name}, &namespaceClass); err != nil {
			logger.Error(err, "unable to fetch NamespaceClass")
			return ctrl.Result{}, err
		}
		networking.Spec = *namespaceClass.Spec.Networking.DeepCopy()
		if err := r.Client.Update(ctx, &networking); err != nil {
			logger.Error(err, "unable to update Networking")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
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
		// ...make sure it's a Networking...
		if owner.APIVersion != ApiGVStr || owner.Kind != "Networking" {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&akuityiov1.NamespaceClass{}).
		Named("namespaceclass").
		Watches(&corev1.Namespace{}, handler.EnqueueRequestsFromMapFunc(r.watchNamespaceResource), builder.WithPredicates(NamespacePredicate)).
		Complete(r)
}

var (
	NamespaceClassOwnerKey = ".metadata.controller"
	ApiGVStr               = akuityiov1.GroupVersion.String()
)

// NamespaceClassLabel is the label used to identify the namespace class that namespace resource refers to.
const NamespaceClassLabel = "namespaceclass.akuity.io/name"

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
			Namespace: namespace.Namespace,
		},
	}}
}
