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

// The keys of the connection Secret: the token's value and the service
// account's user name.
const (
	KeyToken    = "token"
	KeyUsername = "username"
)

// tokenClient is the external client of Token objects. The external name is
// the token's ID. Artifact Keeper shows a token's value only when it is made,
// so a token is never adopted, nor changed in place: a token whose scopes or repositories
// differ, or that expired, reads as missing, and Create replaces it in the
// same reconcile (it makes the new one, revokes the old one, and the new one
// is published in the connection Secret).
type tokenClient struct {
	kube client.Client
	ak   *ak.Client
}

// repositoryIDs returns the IDs of the repositories cr restricts the token
// to: those of repositoryRefs, then those of repositoryKeys. It fails if one
// is missing or a referenced Repository has no key yet.
func (e *tokenClient) repositoryIDs(ctx context.Context, cr *v1alpha1.Token) ([]string, error) {
	p := cr.Spec.ForProvider
	ids := make([]string, 0, len(p.RepositoryRefs)+len(p.RepositoryKeys))
	for i := range p.RepositoryRefs {
		id, err := repositoryID(ctx, e.kube, e.ak, cr.GetNamespace(), &p.RepositoryRefs[i], "")
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	for _, k := range p.RepositoryKeys {
		id, err := repositoryID(ctx, e.kube, e.ak, cr.GetNamespace(), nil, k)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// find returns the first token of the service account saID that match
// accepts, or nil if none does.
func (e *tokenClient) find(ctx context.Context, saID string, match func(ak.Token) bool) (*ak.Token, error) {
	tokens, err := e.ak.ListTokens(ctx, saID)
	if err != nil {
		return nil, err
	}
	for i := range tokens {
		if match(tokens[i]) {
			return &tokens[i], nil
		}
	}
	return nil, nil
}

// Observe finds the token by ID among the account's tokens and records what
// it reports. A token that expired, or whose scopes or repositories differ
// from the spec, reads as missing so that Create replaces it; otherwise it is
// up to date (a new name or expiresInDays waits for the next token).
func (e *tokenClient) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, err := as[v1alpha1.Token](mg)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	id := serverID(cr)
	// The account's ID comes from the ServiceAccount object, except during
	// deletion when it was recorded: the object may be gone by then. If it
	// can't be read during deletion, the token is taken as gone.
	saID := cr.Status.AtProvider.ServiceAccountID
	if saID == "" || !meta.WasDeleted(cr) {
		if saID, _, err = serviceAccount(ctx, e.kube, cr.GetNamespace(), cr.Spec.ForProvider.ServiceAccountRef); err != nil {
			if meta.WasDeleted(cr) {
				return managed.ExternalObservation{ResourceExists: false}, nil
			}
			return managed.ExternalObservation{}, err
		}
	}
	if id == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	have, err := e.find(ctx, saID, func(t ak.Token) bool { return t.ID == id })
	// An account that's gone (404) or a token not in its list: missing.
	if ak.IsNotFound(err) || (err == nil && have == nil) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	cr.Status.AtProvider = v1alpha1.TokenObservation{ID: have.ID, ServiceAccountID: saID, Prefix: have.Prefix,
		ExpiresAt: deref(have.ExpiresAt, ""), RepositoryIDs: have.RepositoryIDs, Scopes: have.Scopes}
	if meta.WasDeleted(cr) {
		return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
	}
	repos, err := e.repositoryIDs(ctx, cr)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	if have.IsExpired || !sameSet(have.Scopes, cr.Spec.ForProvider.Scopes) || !sameSet(have.RepositoryIDs, repos) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	cr.SetConditions(xpv2.Available())
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
}

// Create makes a token for the account, records its ID as the external name,
// revokes the token it replaces and any other of the same name, and returns
// the value and the account's user name as connection details.
func (e *tokenClient) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, err := as[v1alpha1.Token](mg)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	saID, username, err := serviceAccount(ctx, e.kube, cr.GetNamespace(), cr.Spec.ForProvider.ServiceAccountRef)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	// A ServiceAccount object not yet observed has no user name in its
	// status: ask Artifact Keeper.
	if username == "" {
		sa, err := e.ak.GetServiceAccount(ctx, saID)
		if err != nil {
			return managed.ExternalCreation{}, errors.Wrap(err, "cannot get the service account's user name")
		}
		username = sa.Username
	}
	p := cr.Spec.ForProvider
	repos, err := e.repositoryIDs(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	replaced := serverID(cr)
	t, err := e.ak.CreateToken(ctx, saID, ak.CreateToken{Name: p.Name, Scopes: p.Scopes, RepositoryIDs: repos, ExpiresInDays: p.ExpiresInDays})
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot create token")
	}
	meta.SetExternalName(cr, t.ID)
	// Recorded now, not only at the next Observe: Delete needs the account's
	// ID to revoke the token, also when it runs before any Observe.
	cr.Status.AtProvider.ID, cr.Status.AtProvider.ServiceAccountID = t.ID, saID
	// Then the token it replaces (ours, out of date) and any other of this
	// name (Artifact Keeper allows several; their values are lost) are
	// revoked: the old token works until the new one exists.
	tokens, err := e.ak.ListTokens(ctx, saID)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	for _, old := range tokens {
		if old.ID == t.ID || (old.ID != replaced && old.Name != p.Name) {
			continue
		}
		if err := e.ak.RevokeToken(ctx, saID, old.ID); err != nil && !ak.IsNotFound(err) {
			return managed.ExternalCreation{}, errors.Wrap(err, "cannot revoke the token it replaces")
		}
	}
	return managed.ExternalCreation{ConnectionDetails: managed.ConnectionDetails{
		KeyToken:    []byte(t.Token),
		KeyUsername: []byte(username),
	}}, nil
}

// Update does nothing: a token is never changed in place (see tokenClient).
func (e *tokenClient) Update(_ context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	_, err := as[v1alpha1.Token](mg)
	return managed.ExternalUpdate{}, err
}

// Delete revokes the token, with the account's ID recorded in status by
// Create or Observe, else that of the ServiceAccount object. Without a token
// ID, or with the account gone, there is nothing to revoke; a token already
// gone is no error.
func (e *tokenClient) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, err := as[v1alpha1.Token](mg)
	if err != nil {
		return managed.ExternalDelete{}, err
	}
	cr.SetConditions(xpv2.Deleting())
	id := serverID(cr)
	if id == "" {
		return managed.ExternalDelete{}, nil // never created
	}
	saID := cr.Status.AtProvider.ServiceAccountID
	if saID == "" {
		// No ID recorded (a token made by an older version and never
		// observed): ask the ServiceAccount object. If it is gone, so is the
		// account and its tokens with it.
		if saID, _, err = serviceAccount(ctx, e.kube, cr.GetNamespace(), cr.Spec.ForProvider.ServiceAccountRef); err != nil {
			return managed.ExternalDelete{}, nil //nolint:nilerr // nothing left to revoke
		}
	}
	if err := e.ak.RevokeToken(ctx, saID, id); err != nil && !ak.IsNotFound(err) {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot revoke token")
	}
	return managed.ExternalDelete{}, nil
}

// Disconnect does nothing: the client holds no connection.
func (e *tokenClient) Disconnect(context.Context) error { return nil }
