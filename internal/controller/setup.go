// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

// Package controller holds the provider's controllers: one managed
// reconciler per kind (crossplane-runtime), each with an external client
// that calls Artifact Keeper's API, and the ProviderConfig reconcilers.
package controller

import (
	"context"

	xpcontroller "github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/providerconfig"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"brobald.be/provider-artifact-keeper/apis/v1alpha1"
	"brobald.be/provider-artifact-keeper/apis/v1beta1"
	"brobald.be/provider-artifact-keeper/internal/clients"
	"brobald.be/provider-artifact-keeper/internal/clients/ak"
)

// kind is one managed resource kind and how its external client is made.
type kind struct {
	gvk  schema.GroupVersionKind
	obj  client.Object
	make func(kube client.Client, c *ak.Client) managed.ExternalClient
	// serverID: Artifact Keeper assigns the external name (an ID). Otherwise
	// the external name defaults to the object's name (a repository's key).
	serverID bool
}

// kinds lists the managed resource kinds.

func kinds() []kind {
	return []kind{
		{v1alpha1.RepositoryGroupVersionKind, &v1alpha1.Repository{}, func(k client.Client, c *ak.Client) managed.ExternalClient { return &repositoryClient{kube: k, ak: c} }, false},
		{v1alpha1.PermissionGroupVersionKind, &v1alpha1.Permission{}, func(k client.Client, c *ak.Client) managed.ExternalClient { return &permissionClient{kube: k, ak: c} }, true},
		{v1alpha1.OIDCProviderGroupVersionKind, &v1alpha1.OIDCProvider{}, func(k client.Client, c *ak.Client) managed.ExternalClient { return &oidcClient{kube: k, ak: c} }, true},
		{v1alpha1.ServiceAccountGroupVersionKind, &v1alpha1.ServiceAccount{}, func(k client.Client, c *ak.Client) managed.ExternalClient {
			return &serviceAccountClient{kube: k, ak: c}
		}, true},
		{v1alpha1.TokenGroupVersionKind, &v1alpha1.Token{}, func(k client.Client, c *ak.Client) managed.ExternalClient { return &tokenClient{kube: k, ak: c} }, true},
	}
}

// Setup sets up every controller at once, without waiting for CRDs (when
// the provider may not watch them: no safe-start).
func Setup(mgr ctrl.Manager, o xpcontroller.Options) error {
	if err := setupProviderConfigs(mgr, o); err != nil {
		return err
	}
	for _, k := range kinds() {
		if err := setupKind(mgr, o, k); err != nil {
			return err
		}
	}
	return nil
}

// SetupGated starts each controller once its CRDs exist (safe-start: a kind
// is installed only when a ManagedResourceActivationPolicy activates it).
func SetupGated(mgr ctrl.Manager, o xpcontroller.Options) error {
	o.Gate.Register(func() {
		if err := setupProviderConfigs(mgr, o); err != nil {
			mgr.GetLogger().Error(err, "cannot set up the ProviderConfig controllers")
		}
	}, v1beta1.ProviderConfigGroupVersionKind, v1beta1.ClusterProviderConfigGroupVersionKind, v1beta1.ProviderConfigUsageGroupVersionKind)
	for _, k := range kinds() {
		o.Gate.Register(func() {
			if err := setupKind(mgr, o, k); err != nil {
				mgr.GetLogger().Error(err, "cannot set up controller", "gvk", k.gvk.String())
			}
		}, k.gvk)
	}
	return nil
}

func setupKind(mgr ctrl.Manager, o xpcontroller.Options, k kind) error {
	name := managed.ControllerName(k.gvk.GroupKind().String())
	opts := []managed.ReconcilerOption{
		managed.WithExternalConnector(&connector{kube: mgr.GetClient(), make: k.make}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))), //nolint:staticcheck // the runtime's recorder API
		managed.WithPollInterval(o.PollInterval),
		managed.WithManagementPolicies(),
	}
	if k.serverID {
		// No initializers: drops the default one that sets the external name
		// to the object's name, since the ID comes with the creation.
		opts = append(opts, managed.WithInitializers())
	}
	if o.MetricOptions != nil {
		opts = append(opts, managed.WithMetricRecorder(o.MetricOptions.MRMetrics))
	}
	r := managed.NewReconciler(mgr, resource.ManagedKind(k.gvk), opts...)
	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(k.obj).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

func setupProviderConfigs(mgr ctrl.Manager, o xpcontroller.Options) error {
	usage := resource.ProviderConfigKinds{
		Usage:     v1beta1.ProviderConfigUsageGroupVersionKind,
		UsageList: v1beta1.ProviderConfigUsageListGroupVersionKind,
	}
	for _, pc := range []struct {
		gvk schema.GroupVersionKind
		gk  string
		obj client.Object
	}{
		{v1beta1.ProviderConfigGroupVersionKind, v1beta1.ProviderConfigGroupKind, &v1beta1.ProviderConfig{}},
		{v1beta1.ClusterProviderConfigGroupVersionKind, v1beta1.ClusterProviderConfigGroupKind, &v1beta1.ClusterProviderConfig{}},
	} {
		name := providerconfig.ControllerName(pc.gk)
		of := usage
		of.Config = pc.gvk
		if err := ctrl.NewControllerManagedBy(mgr).
			Named(name).
			WithOptions(o.ForControllerRuntime()).
			For(pc.obj).
			Watches(&v1beta1.ProviderConfigUsage{}, &resource.EnqueueRequestForProviderConfig{}).
			Complete(providerconfig.NewReconciler(mgr, of,
				providerconfig.WithLogger(o.Logger.WithValues("controller", name)),
				providerconfig.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))))); err != nil { //nolint:staticcheck // the runtime's recorder API
			return errors.Wrapf(err, "cannot set up %s", name)
		}
	}
	return nil
}

// connector makes a kind's external client with the resource's provider
// config.
type connector struct {
	kube client.Client
	make func(kube client.Client, c *ak.Client) managed.ExternalClient
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	mm, ok := mg.(resource.ModernManaged)
	if !ok {
		return nil, errors.New("not a namespaced managed resource")
	}
	akc, err := clients.Connect(ctx, c.kube, mm)
	if err != nil {
		return nil, err
	}
	return c.make(c.kube, akc), nil
}
