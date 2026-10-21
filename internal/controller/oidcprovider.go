// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"brobald.be/provider-artifact-keeper/apis/v1alpha1"
	"brobald.be/provider-artifact-keeper/internal/clients/ak"
)

type oidcClient struct {
	kube client.Client
	ak   *ak.Client
}

func asOIDC(mg resource.Managed) (*v1alpha1.OIDCProvider, error) {
	cr, ok := mg.(*v1alpha1.OIDCProvider)
	if !ok {
		return nil, errors.New("not an OIDCProvider")
	}
	return cr, nil
}

func (e *oidcClient) secret(ctx context.Context, cr *v1alpha1.OIDCProvider) (string, error) {
	ref := cr.Spec.ForProvider.ClientSecretSecretRef
	s := &corev1.Secret{}
	if err := e.kube.Get(ctx, types.NamespacedName{Namespace: cr.GetNamespace(), Name: ref.Name}, s); err != nil {
		return "", errors.Wrapf(err, "cannot get Secret %s", ref.Name)
	}
	v, ok := s.Data[ref.Key]
	if !ok || len(v) == 0 {
		return "", errors.Errorf("Secret %s has no key %s", ref.Name, ref.Key)
	}
	return strings.TrimSpace(string(v)), nil
}

func hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func request(cr *v1alpha1.OIDCProvider, secret string) ak.OIDCProviderRequest {
	p := cr.Spec.ForProvider
	scopes := p.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}
	am := map[string]any{
		"groups_claim":   orDefault(p.GroupsClaim, "groups"),
		"username_claim": orDefault(p.UsernameClaim, "preferred_username"),
	}
	if p.RedirectURI != "" {
		am["redirect_uri"] = p.RedirectURI
	}
	var admin *string
	if p.AdminGroup != "" {
		am["admin_group"] = p.AdminGroup
		admin = &p.AdminGroup
	}
	return ak.OIDCProviderRequest{
		Name: p.Name, IssuerURL: p.IssuerURL, ClientID: p.ClientID, ClientSecret: secret, Scopes: scopes,
		AttributeMapping: am, IsEnabled: deref(p.Enabled, true), AutoCreateUsers: deref(p.AutoCreateUsers, true),
		PKCEEnabled: deref(p.PKCE, true), MapGroupsToGroups: deref(p.MapGroupsToGroups, true), AdminGroup: admin,
	}
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func oidcUpToDate(have *ak.OIDCProvider, want ak.OIDCProviderRequest) bool {
	if have.Name != want.Name || have.IssuerURL != want.IssuerURL || have.ClientID != want.ClientID ||
		!sameSet(have.Scopes, want.Scopes) || have.IsEnabled != want.IsEnabled || have.AutoCreateUsers != want.AutoCreateUsers ||
		have.PKCEEnabled != want.PKCEEnabled || have.MapGroupsToGroups != want.MapGroupsToGroups || !have.HasSecret {
		return false
	}
	if len(have.AttributeMapping) != len(want.AttributeMapping) {
		return false
	}
	for k, v := range want.AttributeMapping {
		if hv, ok := have.AttributeMapping[k]; !ok || hv != v {
			return false
		}
	}
	return true
}

func (e *oidcClient) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, err := asOIDC(mg)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	var have *ak.OIDCProvider
	adopted := false
	if id := serverID(cr); id != "" {
		have, err = e.ak.GetOIDCProvider(ctx, id)
		if ak.IsNotFound(err) {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		if err != nil {
			return managed.ExternalObservation{}, err
		}
	} else {
		have, err = e.ak.FindOIDCProvider(ctx, cr.Spec.ForProvider.Name)
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
	if meta.WasDeleted(cr) {
		return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
	}
	secret, err := e.secret(ctx, cr)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	cr.Status.AtProvider.ID = have.ID
	cr.SetConditions(xpv2.Available())
	// The secret can't be compared with the server's: its hash is compared
	// with the one recorded when it was last sent.
	upToDate := oidcUpToDate(have, request(cr, "")) && cr.Status.AtProvider.ClientSecretHash == hash(secret)
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate, ResourceLateInitialized: adopted}, nil
}

func (e *oidcClient) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, err := asOIDC(mg)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	secret, err := e.secret(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	p, err := e.ak.CreateOIDCProvider(ctx, request(cr, secret))
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot create OIDC provider")
	}
	meta.SetExternalName(cr, p.ID)
	cr.Status.AtProvider.ClientSecretHash = hash(secret)
	return managed.ExternalCreation{}, nil
}

func (e *oidcClient) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, err := asOIDC(mg)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	secret, err := e.secret(ctx, cr)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	if err := e.ak.UpdateOIDCProvider(ctx, serverID(cr), request(cr, secret)); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "cannot update OIDC provider")
	}
	cr.Status.AtProvider.ClientSecretHash = hash(secret)
	return managed.ExternalUpdate{}, nil
}

func (e *oidcClient) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, err := asOIDC(mg)
	if err != nil {
		return managed.ExternalDelete{}, err
	}
	cr.SetConditions(xpv2.Deleting())
	if id := serverID(cr); id != "" {
		if err := e.ak.DeleteOIDCProvider(ctx, id); err != nil && !ak.IsNotFound(err) {
			return managed.ExternalDelete{}, errors.Wrap(err, "cannot delete OIDC provider")
		}
	}
	return managed.ExternalDelete{}, nil
}

func (e *oidcClient) Disconnect(context.Context) error { return nil }
