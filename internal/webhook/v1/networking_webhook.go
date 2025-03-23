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

package v1

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	akuityiov1 "akuity.io/namespaceclass/api/v1"
)

// nolint:unused
// log is for logging in this package.
var networkinglog = logf.Log.WithName("networking-resource")

// SetupNetworkingWebhookWithManager registers the webhook for Networking in the manager.
func SetupNetworkingWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&akuityiov1.Networking{}).
		WithValidator(&NetworkingCustomValidator{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-akuity-io-v1-networking,mutating=false,failurePolicy=fail,sideEffects=None,groups=akuity.io,resources=networkings,verbs=create;update,versions=v1,name=vnetworking-v1.kb.io,admissionReviewVersions=v1

// NetworkingCustomValidator struct is responsible for validating the Networking resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type NetworkingCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &NetworkingCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Networking.
func (v *NetworkingCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	networking, ok := obj.(*akuityiov1.Networking)
	if !ok {
		return nil, fmt.Errorf("expected a Networking object but got %T", obj)
	}
	networkinglog.Info("Validation for Networking upon creation", "name", networking.GetName())

	if err := validateNetworking(networking); err != nil {
		networkinglog.Error(err, "Validation failed for Networking upon creation", "name", networking.GetName())
		return nil, err
	}
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Networking.
func (v *NetworkingCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	networking, ok := newObj.(*akuityiov1.Networking)
	if !ok {
		return nil, fmt.Errorf("expected a Networking object for the newObj but got %T", newObj)
	}

	if err := validateNetworking(networking); err != nil {
		networkinglog.Error(err, "Validation failed for Networking upon update", "name", networking.GetName())
		return nil, err
	}

	networkinglog.Info("Validation succeeded for Networking upon update", "name", networking.GetName())
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Networking.
func (v *NetworkingCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	networking, ok := obj.(*akuityiov1.Networking)
	if !ok {
		return nil, fmt.Errorf("expected a Networking object but got %T", obj)
	}
	networkinglog.Info("Validation succeeded for Networking upon deletion", "name", networking.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}

func validateNetworking(networking *akuityiov1.Networking) error {
	err := checkDuplicateNetworkPolicies(networking.Spec.NetworkPolicies, field.NewPath("spec"))
	if err != nil {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "akuity.io", Kind: "Networking"},
			networking.Name, err,
		)
	}
	return nil
}

func checkDuplicateNetworkPolicies(networkPolicies []akuityiov1.NetworkingPolicy, p *field.Path) field.ErrorList {
	if len(networkPolicies) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	var allErrs field.ErrorList
	for i, networkPolicy := range networkPolicies {
		if _, ok := seen[networkPolicy.Name]; ok {
			allErrs = append(allErrs, field.Duplicate(
				p.Child(fmt.Sprintf("networkpolicies[%d]", i), "name"),
				networkPolicy.Name),
			)
		}
		seen[networkPolicy.Name] = struct{}{}
	}

	if len(allErrs) == 0 {
		return nil
	}
	return allErrs
}
