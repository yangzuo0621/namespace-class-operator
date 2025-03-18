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

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	akuityiov1 "akuity.io/namespaceclass/api/v1"
)

// NetworkingReconciler reconciles a Networking object
type NetworkingReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=akuity.io,resources=networkings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=akuity.io,resources=networkings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=akuity.io,resources=networkings/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Networking object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *NetworkingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var networking akuityiov1.Networking
	if err := r.Get(ctx, req.NamespacedName, &networking); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var networkPolicyList networkingv1.NetworkPolicyList
	if err := r.List(ctx, &networkPolicyList, client.MatchingFields{NamespaceClassOwnerKey: req.Name}); err != nil {
		return ctrl.Result{}, err
	}

	expected := make(map[string]*akuityiov1.NetworkingPolicy, len(networking.Spec.NetworkPolicies))
	for _, p := range networking.Spec.NetworkPolicies {
		expected[p.Name] = &p
	}
	actual := make(map[string]*networkingv1.NetworkPolicy, len(networkPolicyList.Items))
	for _, p := range networkPolicyList.Items {
		actual[p.Name] = &p
	}

	// update or create network policies
	for name, policy := range expected {
		if networkPolicy, ok := actual[name]; ok {
			networkPolicy.Spec = *policy.Spec.DeepCopy()
			if err := r.Update(ctx, networkPolicy); err != nil {
				logger.Error(err, "update network policy failed")
			}
		} else {
			if err := r.createNetworkPolicy(ctx, &networking, req.Namespace, policy); err != nil {
				logger.Error(err, "create network policy failed")
			}
		}
	}

	// delete network policies
	for name, policy := range actual {
		if _, ok := expected[name]; ok {
			continue
		}

		err := r.Delete(ctx, policy, client.PropagationPolicy(metav1.DeletePropagationBackground))
		if err != nil {
			logger.Error(err, "delete network policy failed")
		}
	}

	return ctrl.Result{}, nil
}

func (r *NetworkingReconciler) createNetworkPolicy(ctx context.Context, owner *akuityiov1.Networking, namespace string, networkPolicy *akuityiov1.NetworkingPolicy) error {
	object := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      networkPolicy.Name,
			Namespace: namespace,
		},
		Spec: *networkPolicy.Spec.DeepCopy(),
	}
	err := ctrl.SetControllerReference(owner, &object, r.Scheme)
	if err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}
	err = r.Create(ctx, &object)
	if err != nil {
		return fmt.Errorf("failed to create network policy: %w", err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &networkingv1.NetworkPolicy{}, NamespaceClassOwnerKey, func(rawObj client.Object) []string {
		// grab the NetworkPolicy object, extract the owner...
		networkPolicy := rawObj.(*networkingv1.NetworkPolicy)
		owner := metav1.GetControllerOf(networkPolicy)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != ApiGVStr || owner.Kind != "Networking" {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&akuityiov1.Networking{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Named("networking").
		Complete(r)
}
