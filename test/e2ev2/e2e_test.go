package e2ev2

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	akuityiov1 "akuity.io/namespaceclass/api/v1"
)

var (
	frontEndPolicy = networkingv1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app": "front-end",
			},
		},
		PolicyTypes: []networkingv1.PolicyType{
			networkingv1.PolicyTypeIngress,
			networkingv1.PolicyTypeEgress,
		},
	}

	backEndPolicy = networkingv1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app": "back-end",
			},
		},
		PolicyTypes: []networkingv1.PolicyType{
			networkingv1.PolicyTypeIngress,
			networkingv1.PolicyTypeEgress,
		},
	}

	publicNetworkClass = akuityiov1.NamespaceClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "public-network",
		},
		Spec: akuityiov1.NamespaceClassSpec{
			Networking: akuityiov1.NetworkingSpec{
				NetworkPolicies: []akuityiov1.NetworkingPolicy{
					{
						Name: "front-end",
						Spec: frontEndPolicy,
					},
				},
			},
		},
	}

	privateNetworkClass = akuityiov1.NamespaceClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "private-network",
		},
		Spec: akuityiov1.NamespaceClassSpec{
			Networking: akuityiov1.NetworkingSpec{
				NetworkPolicies: []akuityiov1.NetworkingPolicy{
					{
						Name: "back-end",
						Spec: backEndPolicy,
					},
				},
			},
		},
	}
)

func TestNamespaceClass(t *testing.T) {
	feature := features.New("NamespaceClass").
		Assess("Create NamespaceClass", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			resource := MustGetNamespaceClassResources(ctx)
			err := resource.Create(ctx, &publicNetworkClass)
			assert.NoError(t, err)

			err = resource.Create(ctx, &privateNetworkClass)
			assert.NoError(t, err)

			var namespaceClassList akuityiov1.NamespaceClassList
			err = wait.For(conditions.New(resource).ResourceListN(&namespaceClassList, 2), wait.WithInterval(1*time.Second), wait.WithTimeout(10*time.Second))
			assert.NoError(t, err)

			return ctx
		}).
		Assess("Create Namespace", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			namespace := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "web-portal",
					Labels: map[string]string{NamespaceClassLabel: "public-network"},
				},
			}

			client, err := cfg.NewClient()
			assert.NoError(t, err)
			err = client.Resources().Create(ctx, &namespace)
			assert.NoError(t, err)

			resource := MustGetNamespaceClassResources(ctx)

			networking := akuityiov1.Networking{ObjectMeta: metav1.ObjectMeta{Name: "public-network", Namespace: "web-portal"}}
			err = wait.For(conditions.New(resource).ResourceMatch(&networking, func(object k8s.Object) bool {
				obj := object.(*akuityiov1.Networking)
				return len(obj.Spec.NetworkPolicies) == 1 && obj.Spec.NetworkPolicies[0].Name == "front-end"
			}), wait.WithInterval(1*time.Second), wait.WithTimeout(10*time.Second))
			assert.NoError(t, err)

			networkPolicy := networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "front-end", Namespace: "web-portal"}}
			err = wait.For(conditions.New(client.Resources()).ResourceMatch(&networkPolicy, func(object k8s.Object) bool {
				obj := object.(*networkingv1.NetworkPolicy)
				return obj.Spec.PodSelector.MatchLabels["app"] == "front-end"
			}), wait.WithInterval(1*time.Second), wait.WithTimeout(10*time.Second))
			assert.NoError(t, err)

			return ctx
		}).
		Assess("Change Namespace Class of Namespace Resource", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			client, err := cfg.NewClient()
			assert.NoError(t, err)
			var namespace corev1.Namespace
			err = client.Resources().Get(ctx, "web-portal", "", &namespace)
			assert.NoError(t, err)
			assert.Equal(t, "public-network", namespace.Labels[NamespaceClassLabel])
			namespace.Labels[NamespaceClassLabel] = "private-network"
			err = client.Resources().Update(ctx, &namespace)
			assert.NoError(t, err)
			err = client.Resources().Get(ctx, "web-portal", "", &namespace)
			assert.NoError(t, err)
			assert.Equal(t, "private-network", namespace.Labels[NamespaceClassLabel])

			resource := MustGetNamespaceClassResources(ctx)

			networking := akuityiov1.Networking{ObjectMeta: metav1.ObjectMeta{Name: "public-network", Namespace: "web-portal"}}
			err = wait.For(conditions.New(resource).ResourceDeleted(&networking), wait.WithInterval(1*time.Second), wait.WithTimeout(10*time.Second))
			assert.NoError(t, err)

			networkPolicy := networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "front-end", Namespace: "web-portal"}}
			err = wait.For(conditions.New(client.Resources()).ResourceDeleted(&networkPolicy), wait.WithInterval(1*time.Second), wait.WithTimeout(10*time.Second))
			assert.NoError(t, err)

			networking = akuityiov1.Networking{ObjectMeta: metav1.ObjectMeta{Name: "private-network", Namespace: "web-portal"}}
			err = wait.For(conditions.New(resource).ResourceMatch(&networking, func(object k8s.Object) bool {
				obj := object.(*akuityiov1.Networking)
				return len(obj.Spec.NetworkPolicies) == 1 && obj.Spec.NetworkPolicies[0].Name == "back-end"
			}), wait.WithInterval(1*time.Second), wait.WithTimeout(10*time.Second))
			assert.NoError(t, err)

			networkPolicy = networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "back-end", Namespace: "web-portal"}}
			err = wait.For(conditions.New(client.Resources()).ResourceMatch(&networkPolicy, func(object k8s.Object) bool {
				obj := object.(*networkingv1.NetworkPolicy)
				return obj.Spec.PodSelector.MatchLabels["app"] == "back-end"
			}), wait.WithInterval(1*time.Second), wait.WithTimeout(10*time.Second))
			assert.NoError(t, err)

			return ctx
		}).
		Assess("Update Namespace Class", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			resource := MustGetNamespaceClassResources(ctx)
			var namespaceClass akuityiov1.NamespaceClass
			err := resource.Get(ctx, "private-network", "", &namespaceClass)
			assert.NoError(t, err)
			assert.Len(t, namespaceClass.Spec.Networking.NetworkPolicies, 1)
			assert.Equal(t, "back-end", namespaceClass.Spec.Networking.NetworkPolicies[0].Name)

			namespaceClass.Spec.Networking.NetworkPolicies[0].Spec.PodSelector = metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "database",
				},
			}
			namespaceClass.Spec.Networking.NetworkPolicies = append(namespaceClass.Spec.Networking.NetworkPolicies, akuityiov1.NetworkingPolicy{
				Name: "new-policy",
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"type": "new-app"},
					},
					PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress, networkingv1.PolicyTypeIngress},
				},
			})
			err = resource.Update(ctx, &namespaceClass)
			assert.NoError(t, err)

			networking := akuityiov1.Networking{ObjectMeta: metav1.ObjectMeta{Name: "private-network", Namespace: "web-portal"}}
			err = wait.For(conditions.New(resource).ResourceMatch(&networking, func(object k8s.Object) bool {
				obj := object.(*akuityiov1.Networking)
				return len(obj.Spec.NetworkPolicies) == 2 && obj.Spec.NetworkPolicies[0].Name == "back-end" && obj.Spec.NetworkPolicies[1].Name == "new-policy"
			}), wait.WithInterval(1*time.Second), wait.WithTimeout(10*time.Second))
			assert.NoError(t, err)

			client, err := cfg.NewClient()
			assert.NoError(t, err)
			networkPolicy := networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "back-end", Namespace: "web-portal"}}
			err = wait.For(conditions.New(client.Resources()).ResourceMatch(&networkPolicy, func(object k8s.Object) bool {
				obj := object.(*networkingv1.NetworkPolicy)
				return obj.Spec.PodSelector.MatchLabels["app"] == "database"
			}), wait.WithInterval(1*time.Second), wait.WithTimeout(10*time.Second))
			assert.NoError(t, err)

			networkPolicy = networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "new-policy", Namespace: "web-portal"}}
			err = wait.For(conditions.New(client.Resources()).ResourceMatch(&networkPolicy, func(object k8s.Object) bool {
				obj := object.(*networkingv1.NetworkPolicy)
				return obj.Spec.PodSelector.MatchLabels["type"] == "new-app"
			}), wait.WithInterval(1*time.Second), wait.WithTimeout(10*time.Second))
			assert.NoError(t, err)

			return ctx
		}).
		Assess("Delete Namespace Class", func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			resource := MustGetNamespaceClassResources(ctx)
			var namespaceClass akuityiov1.NamespaceClass
			err := resource.Get(ctx, "private-network", "", &namespaceClass)
			assert.NoError(t, err)
			assert.Len(t, namespaceClass.Spec.Networking.NetworkPolicies, 2)
			assert.Equal(t, "back-end", namespaceClass.Spec.Networking.NetworkPolicies[0].Name)
			assert.Equal(t, "new-policy", namespaceClass.Spec.Networking.NetworkPolicies[1].Name)

			err = resource.Delete(ctx, &namespaceClass)
			assert.NoError(t, err)

			wait.For(conditions.New(resource).ResourceDeleted(&namespaceClass), wait.WithInterval(1*time.Second), wait.WithTimeout(10*time.Second))

			networking := akuityiov1.Networking{ObjectMeta: metav1.ObjectMeta{Name: "private-network", Namespace: "web-portal"}}
			err = wait.For(conditions.New(resource).ResourceDeleted(&networking), wait.WithInterval(1*time.Second), wait.WithTimeout(10*time.Second))
			assert.NoError(t, err)

			client, err := c.NewClient()
			assert.NoError(t, err)
			networkPolicy := networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "back-end", Namespace: "web-portal"}}
			err = wait.For(conditions.New(client.Resources()).ResourceDeleted(&networkPolicy), wait.WithInterval(1*time.Second), wait.WithTimeout(10*time.Second))
			assert.NoError(t, err)

			networkPolicy = networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "new-policy", Namespace: "web-portal"}}
			err = wait.For(conditions.New(client.Resources()).ResourceDeleted(&networkPolicy), wait.WithInterval(1*time.Second), wait.WithTimeout(10*time.Second))
			assert.NoError(t, err)

			return ctx
		}).
		Feature()

	testenv.Test(t, feature)
}
