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

// serviceAccountClient manages a ServiceAccount (svc-<name>): the external
// name is its ID; without one, svc-<name> is adopted if it exists.
type serviceAccountClient struct {
	kube client.Client
	ak   *ak.Client
}

func asServiceAccount(mg resource.Managed) (*v1alpha1.ServiceAccount, error) {
	cr, ok := mg.(*v1alpha1.ServiceAccount)
	if !ok {
		return nil, errors.New("not a ServiceAccount")
	}
	return cr, nil
}

func (e *serviceAccountClient) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, err := asServiceAccount(mg)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	var have *ak.ServiceAccount
	adopted := false
	if id := serverID(cr); id != "" {
		have, err = e.ak.GetServiceAccount(ctx, id)
		if ak.IsNotFound(err) {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		if err != nil {
			return managed.ExternalObservation{}, err
		}
	} else {
		have, err = e.ak.FindServiceAccount(ctx, "svc-"+cr.Spec.ForProvider.Name)
		if err != nil {
			return managed.ExternalObservation{}, err
		}
		if have == nil {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		// Adopted: saved through the late initialization (see permission.go).
		meta.SetExternalName(cr, have.ID)
		adopted = true
	}
	cr.Status.AtProvider = v1alpha1.ServiceAccountObservation{ID: have.ID, Username: have.Username}
	cr.SetConditions(xpv2.Available())
	// Only activity is managed (the description is set at creation). Being
	// deleted, it is reported up to date so that no update precedes the
	// deletion.
	upToDate := have.IsActive == deref(cr.Spec.ForProvider.Active, true)
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate || meta.WasDeleted(cr), ResourceLateInitialized: adopted}, nil
}

func (e *serviceAccountClient) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, err := asServiceAccount(mg)
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

func (e *serviceAccountClient) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, err := asServiceAccount(mg)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	active := deref(cr.Spec.ForProvider.Active, true)
	return managed.ExternalUpdate{}, errors.Wrap(e.ak.UpdateServiceAccount(ctx, serverID(cr), nil, &active), "cannot update service account")
}

func (e *serviceAccountClient) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, err := asServiceAccount(mg)
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

func (e *serviceAccountClient) Disconnect(context.Context) error { return nil }
