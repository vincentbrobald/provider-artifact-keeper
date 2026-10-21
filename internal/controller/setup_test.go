// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package controller

import (
	"testing"

	xpcontroller "github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"
	"github.com/crossplane/crossplane-runtime/v2/pkg/gate"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"brobald.be/provider-artifact-keeper/apis"
	"brobald.be/provider-artifact-keeper/apis/v1beta1"
)

// manager builds a manager that is never started: setting controllers up
// doesn't contact the API server.
func manager(t *testing.T) ctrl.Manager {
	t.Helper()
	s := runtime.NewScheme()
	if err := apis.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	mgr, err := ctrl.NewManager(&rest.Config{Host: "https://127.0.0.1:1"}, ctrl.Options{
		Scheme: s, Metrics: metricsserver.Options{BindAddress: "0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}

func options() xpcontroller.Options {
	return xpcontroller.Options{Logger: logging.NewNopLogger(), GlobalRateLimiter: ratelimiter.NewGlobal(1),
		MaxConcurrentReconciles: 1, Features: &feature.Flags{}}
}

func TestSetup(t *testing.T) {
	if err := Setup(manager(t), options()); err != nil {
		t.Fatal(err)
	}
}

func TestSetupGated(t *testing.T) {
	o := options()
	g := new(gate.Gate[schema.GroupVersionKind])
	o.Gate = g
	if err := SetupGated(manager(t), o); err != nil {
		t.Fatal(err)
	}
	g.Set(v1beta1.ProviderConfigGroupVersionKind, true)
	g.Set(v1beta1.ClusterProviderConfigGroupVersionKind, true)
	g.Set(v1beta1.ProviderConfigUsageGroupVersionKind, true)
	for _, k := range kinds() {
		g.Set(k.gvk, true)
	}
}
