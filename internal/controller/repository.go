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

// repositoryClient manages a Repository: its key is the external name.
type repositoryClient struct {
	kube client.Client
	ak   *ak.Client
}

func asRepository(mg resource.Managed) (*v1alpha1.Repository, error) {
	cr, ok := mg.(*v1alpha1.Repository)
	if !ok {
		return nil, errors.New("not a Repository")
	}
	return cr, nil
}

// desiredMembers returns the member keys, in priority order.
func (e *repositoryClient) desiredMembers(ctx context.Context, cr *v1alpha1.Repository) ([]string, error) {
	keys := make([]string, 0, len(cr.Spec.ForProvider.Members))
	for _, m := range cr.Spec.ForProvider.Members {
		k, err := repositoryKey(ctx, e.kube, cr.GetNamespace(), m.RepositoryRef, m.Key)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// displayName is the repository's display name: spec name, else the key.
func displayName(cr *v1alpha1.Repository) string {
	if cr.Spec.ForProvider.Name != "" {
		return cr.Spec.ForProvider.Name
	}
	return meta.GetExternalName(cr)
}

func (e *repositoryClient) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, err := asRepository(mg)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	key := meta.GetExternalName(cr)
	r, err := e.ak.GetRepository(ctx, key)
	if ak.IsNotFound(err) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	if meta.WasDeleted(cr) {
		return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
	}
	p := cr.Spec.ForProvider
	if r.Format != p.Format || r.RepoType != p.Type {
		return managed.ExternalObservation{}, errors.Errorf("repository %s is %s/%s, not %s/%s (format and type can't change)",
			key, r.Format, r.RepoType, p.Format, p.Type)
	}
	cr.Status.AtProvider = v1alpha1.RepositoryObservation{ID: r.ID, Format: r.Format, Type: r.RepoType, StorageUsedBytes: r.StorageUsedBytes}

	// The quota is compared only when set: unset leaves the server's as it
	// is (Update doesn't send it either).
	upToDate := r.Name == displayName(cr) &&
		deref(r.Description, "") == p.Description &&
		r.AllowAnonymousAccess == deref(p.Public, false) &&
		(p.QuotaBytes == nil || (r.QuotaBytes != nil && *r.QuotaBytes == *p.QuotaBytes))
	if p.Type == "virtual" {
		have, err := e.ak.GetMembers(ctx, key)
		if err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, "cannot get members")
		}
		want, err := e.desiredMembers(ctx, cr)
		if err != nil {
			return managed.ExternalObservation{}, err
		}
		got := make([]string, 0, len(have))
		for _, m := range have {
			got = append(got, m.Key)
		}
		cr.Status.AtProvider.Members = got
		// In order: the order is the members' priority.
		upToDate = upToDate && equalOrdered(got, want)
	}
	cr.SetConditions(xpv2.Available())
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate}, nil
}

func (e *repositoryClient) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, err := asRepository(mg)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	p := cr.Spec.ForProvider
	key := meta.GetExternalName(cr)
	if _, err := e.ak.CreateRepository(ctx, ak.CreateRepository{
		Key: key, Name: displayName(cr), Description: p.Description, Format: p.Format, RepoType: p.Type,
		UpstreamURL: p.UpstreamURL, AllowAnonymousAccess: deref(p.Public, false), QuotaBytes: p.QuotaBytes,
	}); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot create repository")
	}
	return managed.ExternalCreation{}, e.setMembers(ctx, cr)
}

func (e *repositoryClient) setMembers(ctx context.Context, cr *v1alpha1.Repository) error {
	if cr.Spec.ForProvider.Type != "virtual" {
		return nil
	}
	want, err := e.desiredMembers(ctx, cr)
	if err != nil {
		return err
	}
	m := make([]ak.Member, 0, len(want))
	for i, k := range want {
		m = append(m, ak.Member{Key: k, Priority: i + 1})
	}
	return errors.Wrap(e.ak.SetMembers(ctx, meta.GetExternalName(cr), m), "cannot set members")
}

func (e *repositoryClient) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, err := asRepository(mg)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	key := meta.GetExternalName(cr)
	// The update must carry the current curation action (ak.UpdateRepository).
	r, err := e.ak.GetRepository(ctx, key)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	p := cr.Spec.ForProvider
	name, public := displayName(cr), deref(p.Public, false)
	if err := e.ak.UpdateRepository(ctx, key, ak.UpdateRepository{
		Name: &name, Description: &p.Description, AllowAnonymousAccess: &public, QuotaBytes: p.QuotaBytes,
		CurationDefaultAction: r.CurationDefaultMode,
	}); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot update repository")
	}
	return managed.ExternalUpdate{}, e.setMembers(ctx, cr)
}

func (e *repositoryClient) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, err := asRepository(mg)
	if err != nil {
		return managed.ExternalDelete{}, err
	}
	cr.SetConditions(xpv2.Deleting())
	if err := e.ak.DeleteRepository(ctx, meta.GetExternalName(cr)); err != nil && !ak.IsNotFound(err) {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot delete repository")
	}
	return managed.ExternalDelete{}, nil
}

func (e *repositoryClient) Disconnect(context.Context) error { return nil }

// equalOrdered tells whether a and b hold the same strings in the same order.
func equalOrdered(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
