// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package controller

import (
	"context"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"brobald.be/provider-artifact-keeper/apis/v1alpha1"
	"brobald.be/provider-artifact-keeper/internal/clients/ak"
)

// serviceAccountClient is the external client of ServiceAccount objects. The
// external name is the account's ID.
type serviceAccountClient struct {
	kube client.Client
	ak   *ak.Client
}

// Observe reads the account by ID or, without one, finds svc-<name> and
// adopts it, and records its ID and user name. Only the active flag is
// compared (the description is set at creation only); while cr is being
// deleted it reads as up to date.
func (e *serviceAccountClient) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, err := as[v1alpha1.ServiceAccount](mg)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	have, adopted, err := getOrAdopt(ctx, cr, e.ak.GetServiceAccount, func() (*ak.ServiceAccount, error) {
		return e.ak.FindServiceAccount(ctx, "svc-"+cr.Spec.ForProvider.Name)
	}, func(sa *ak.ServiceAccount) string { return sa.ID })
	if err != nil || have == nil {
		return managed.ExternalObservation{ResourceExists: false}, err
	}
	cr.Status.AtProvider = v1alpha1.ServiceAccountObservation{ID: have.ID, Username: have.Username}
	cr.SetConditions(xpv2.Available())
	upToDate := have.IsActive == deref(cr.Spec.ForProvider.Active, true)
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate, ResourceLateInitialized: adopted}, nil
}

// Create creates svc-<name> and records its ID as the external name.
func (e *serviceAccountClient) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, err := as[v1alpha1.ServiceAccount](mg)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	sa, err := e.ak.CreateServiceAccount(ctx, cr.Spec.ForProvider.Name, cr.Spec.ForProvider.Description)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot create service account")
	}
	meta.SetExternalName(cr, sa.ID)
	return managed.ExternalCreation{}, nil
}

// Update sets the active flag; the display name is left as it is.
func (e *serviceAccountClient) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, err := as[v1alpha1.ServiceAccount](mg)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	active := deref(cr.Spec.ForProvider.Active, true)
	return managed.ExternalUpdate{}, errors.Wrap(e.ak.UpdateServiceAccount(ctx, serverID(cr), nil, &active), "cannot update service account")
}

// Delete deletes the account; without an ID, or if it is already gone, there
// is nothing to do.
func (e *serviceAccountClient) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, err := as[v1alpha1.ServiceAccount](mg)
	if err != nil {
		return managed.ExternalDelete{}, err
	}
	cr.SetConditions(xpv2.Deleting())
	if id := serverID(cr); id != "" {
		if err := e.ak.DeleteServiceAccount(ctx, id); err != nil && !ak.IsNotFound(err) {
			return managed.ExternalDelete{}, errors.Wrap(err, "cannot delete service account")
		}
	}
	return managed.ExternalDelete{}, nil
}

// Disconnect does nothing: the client holds no connection.
func (e *serviceAccountClient) Disconnect(context.Context) error { return nil }
