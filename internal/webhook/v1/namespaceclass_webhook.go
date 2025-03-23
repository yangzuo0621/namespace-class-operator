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
var namespaceclasslog = logf.Log.WithName("namespaceclass-resource")

// SetupNamespaceClassWebhookWithManager registers the webhook for NamespaceClass in the manager.
func SetupNamespaceClassWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&akuityiov1.NamespaceClass{}).
		WithValidator(&NamespaceClassCustomValidator{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-akuity-io-v1-namespaceclass,mutating=false,failurePolicy=fail,sideEffects=None,groups=akuity.io,resources=namespaceclasses,verbs=create;update,versions=v1,name=vnamespaceclass-v1.kb.io,admissionReviewVersions=v1

// NamespaceClassCustomValidator struct is responsible for validating the NamespaceClass resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type NamespaceClassCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &NamespaceClassCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type NamespaceClass.
func (v *NamespaceClassCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	namespaceclass, ok := obj.(*akuityiov1.NamespaceClass)
	if !ok {
		return nil, fmt.Errorf("expected a NamespaceClass object but got %T", obj)
	}

	if err := validateNamespaceClass(namespaceclass); err != nil {
		namespaceclasslog.Error(err, "Validation failed for NamespaceClass upon creation", "name", namespaceclass.GetName())
		return nil, err
	}

	namespaceclasslog.Info("Validation succeeded for NamespaceClass upon creation", "name", namespaceclass.GetName())
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type NamespaceClass.
func (v *NamespaceClassCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	namespaceclass, ok := newObj.(*akuityiov1.NamespaceClass)
	if !ok {
		return nil, fmt.Errorf("expected a NamespaceClass object for the newObj but got %T", newObj)
	}

	if err := validateNamespaceClass(namespaceclass); err != nil {
		namespaceclasslog.Error(err, "Validation failed for NamespaceClass upon update", "name", namespaceclass.GetName())
		return nil, err
	}

	namespaceclasslog.Info("Validation succeeded for NamespaceClass upon update", "name", namespaceclass.GetName())
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type NamespaceClass.
func (v *NamespaceClassCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	namespaceclass, ok := obj.(*akuityiov1.NamespaceClass)
	if !ok {
		return nil, fmt.Errorf("expected a NamespaceClass object but got %T", obj)
	}
	namespaceclasslog.Info("Validation for NamespaceClass upon deletion", "name", namespaceclass.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}

func validateNamespaceClass(namespaceclass *akuityiov1.NamespaceClass) error {
	err := checkDuplicateNetworkPolicies(namespaceclass.Spec.Networking.NetworkPolicies, field.NewPath("spec", "networking"))
	if err != nil {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "akuity.io", Kind: "NamespaceClass"},
			namespaceclass.Name, err,
		)
	}
	return nil
}
