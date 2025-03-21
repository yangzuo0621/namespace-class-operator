package e2ev2

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"

	ctrl "sigs.k8s.io/controller-runtime"

	"akuity.io/namespaceclass/test/utils"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kind"

	akuityiov1 "akuity.io/namespaceclass/api/v1"
)

const NamespaceClassLabel = "namespaceclass.akuity.io/name"

var (
	testenv env.Environment
	e2eLog  = ctrl.Log.WithName("e2e-test")
)

func TestMain(m *testing.M) {

	testenv = env.New()
	kindClusterName := envconf.RandomName("test-cluster", 16)
	namespace := envconf.RandomName("testns", 16)

	// Use pre-defined environment funcs to create a kind cluster prior to test run
	testenv.Setup(
		// envfuncs.CreateCluster(kind.NewProvider().WithOpts(kind.WithImage("kindest/node:v1.31.2")), kindClusterName),
		envfuncs.CreateCluster(kind.NewProvider(), kindClusterName),
		envfuncs.CreateNamespace(namespace),
		InstallManager(kindClusterName),
	)

	// Use pre-defined environment funcs to teardown kind cluster after tests
	testenv.Finish(
		envfuncs.DeleteNamespace(namespace),
		envfuncs.DestroyCluster(kindClusterName),
	)

	// launch package tests
	os.Exit(testenv.Run(m))
}

func InstallManager(kindClusterName string) env.Func {
	projectImage := "example.com/namespace-class-operator:v0.0.1"

	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		// build docker image
		cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectImage))
		e2eLog.Info("building image", "cmd", cmd)
		if _, err := utils.Run(cmd); err != nil {
			return ctx, fmt.Errorf("install manager func: %w", err)
		}

		// load image to kind cluster
		e2eLog.Info("loading image to kind cluster", "image", projectImage)
		os.Setenv("KIND_CLUSTER", kindClusterName)
		if err := utils.LoadImageToKindClusterWithName(projectImage); err != nil {
			return ctx, fmt.Errorf("install manager func: %w", err)
		}

		// install namespace class crds
		cmd = exec.Command("make", "install")
		e2eLog.Info("installing manager", "cmd", cmd)
		if _, err := utils.Run(cmd); err != nil {
			return ctx, fmt.Errorf("install manager func: %w", err)
		}

		// deploy manager
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		e2eLog.Info("deploying manager", "cmd", cmd)
		if _, err := utils.Run(cmd); err != nil {
			return ctx, fmt.Errorf("install manager func: %w", err)
		}

		e2eLog.Info("manager installed")

		// create resources and add to api scheme
		r, err := resources.New(cfg.Client().RESTConfig())
		if err != nil {
			return ctx, fmt.Errorf("install manager func: %w", err)
		}
		akuityiov1.AddToScheme(r.GetScheme())

		return context.WithValue(ctx, NamespaceClassResources, r), nil
	}
}

type NamespaceClassContextKey string

const NamespaceClassResources NamespaceClassContextKey = "NamespaceClassResources"

func MustGetNamespaceClassResources(ctx context.Context) *resources.Resources {
	r, ok := ctx.Value(NamespaceClassResources).(*resources.Resources)
	if !ok {
		panic("NamespaceClassResources not found in context")
	}
	return r
}
