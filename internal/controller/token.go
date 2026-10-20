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

// Connection Secret keys of a Token.
const (
	KeyToken    = "token"
	KeyUsername = "username"
)

// tokenClient manages a Token. Artifact Keeper shows a token's value only
// when it is made, so a token is never adopted: one of the same name
// without the external name is revoked and made anew. A token whose scopes
// or repositories differ, or that expired, is revoked (Update); the next
// reconcile finds it gone and makes a new one, published in the connection
// Secret.
type tokenClient struct {
	kube client.Client
	ak   *ak.Client
}

func asToken(mg resource.Managed) (*v1alpha1.Token, error) {
	cr, ok := mg.(*v1alpha1.Token)
	if !ok {
		return nil, errors.New("not a Token")
	}
	return cr, nil
}

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

func (e *tokenClient) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, err := asToken(mg)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	id := serverID(cr)
	saID := cr.Status.AtProvider.ServiceAccountID
	if saID == "" || !meta.WasDeleted(cr) {
		if saID, _, err = serviceAccount(ctx, e.kube, cr.GetNamespace(), cr.Spec.ForProvider.ServiceAccountRef); err != nil {
			if meta.WasDeleted(cr) {
				// The account is gone, and its tokens with it.
				return managed.ExternalObservation{ResourceExists: false}, nil
			}
			return managed.ExternalObservation{}, err
		}
	}
	if id == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	have, err := e.find(ctx, saID, func(t ak.Token) bool { return t.ID == id })
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
	cr.SetConditions(xpv2.Available())
	upToDate := !have.IsExpired && sameSet(have.Scopes, cr.Spec.ForProvider.Scopes) && sameSet(have.RepositoryIDs, repos)
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate}, nil
}

func (e *tokenClient) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, err := asToken(mg)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	saID, username, err := serviceAccount(ctx, e.kube, cr.GetNamespace(), cr.Spec.ForProvider.ServiceAccountRef)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	p := cr.Spec.ForProvider
	// A token of this name without our record of it: its value is lost.
	if old, err := e.find(ctx, saID, func(t ak.Token) bool { return t.Name == p.Name }); err != nil {
		return managed.ExternalCreation{}, err
	} else if old != nil {
		if err := e.ak.RevokeToken(ctx, saID, old.ID); err != nil && !ak.IsNotFound(err) {
			return managed.ExternalCreation{}, errors.Wrap(err, "cannot revoke the token of the same name")
		}
	}
	repos, err := e.repositoryIDs(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	t, err := e.ak.CreateToken(ctx, saID, ak.CreateToken{Name: p.Name, Scopes: p.Scopes, RepositoryIDs: repos, ExpiresInDays: p.ExpiresInDays})
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot create token")
	}
	meta.SetExternalName(cr, t.ID)
	return managed.ExternalCreation{ConnectionDetails: managed.ConnectionDetails{
		KeyToken:    []byte(t.Token),
		KeyUsername: []byte(username),
	}}, nil
}

// Update revokes the token; the next reconcile makes a new one.
func (e *tokenClient) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, err := asToken(mg)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	err = e.ak.RevokeToken(ctx, cr.Status.AtProvider.ServiceAccountID, serverID(cr))
	if err != nil && !ak.IsNotFound(err) {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot revoke token")
	}
	return managed.ExternalUpdate{}, nil
}

func (e *tokenClient) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, err := asToken(mg)
	if err != nil {
		return managed.ExternalDelete{}, err
	}
	cr.SetConditions(xpv2.Deleting())
	if id := serverID(cr); id != "" && cr.Status.AtProvider.ServiceAccountID != "" {
		if err := e.ak.RevokeToken(ctx, cr.Status.AtProvider.ServiceAccountID, id); err != nil && !ak.IsNotFound(err) {
			return managed.ExternalDelete{}, errors.Wrap(err, "cannot revoke token")
		}
	}
	return managed.ExternalDelete{}, nil
}

func (e *tokenClient) Disconnect(context.Context) error { return nil }
