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

// permissionClient is the external client of Permission objects. The
// external name is the permission's ID.
type permissionClient struct {
	kube client.Client
	ak   *ak.Client
}

// asPermission returns mg as a Permission, or an error if it is another kind.
func asPermission(mg resource.Managed) (*v1alpha1.Permission, error) {
	cr, ok := mg.(*v1alpha1.Permission)
	if !ok {
		return nil, errors.New("not a Permission")
	}
	return cr, nil
}

// desired returns the permission cr asks for, with the IDs of its repository
// and principal looked up. It fails if either doesn't exist (yet), e.g. a
// group mirrored from the OIDC login before its first member signed in.
func (e *permissionClient) desired(ctx context.Context, cr *v1alpha1.Permission) (ak.Permission, error) {
	p := cr.Spec.ForProvider
	out := ak.Permission{TargetType: "repository", Actions: p.Actions}
	id, err := repositoryID(ctx, e.kube, e.ak, cr.GetNamespace(), p.RepositoryRef, p.RepositoryKey)
	if err != nil {
		return out, err
	}
	out.TargetID = id
	switch pr := p.Principal; {
	case pr.Group != "":
		g, err := e.ak.FindGroup(ctx, pr.Group)
		if err != nil {
			return out, errors.Wrapf(err, "cannot look up group %s", pr.Group)
		}
		if g == nil {
			return out, errors.Errorf("group %s doesn't exist yet (a group from the OIDC login appears at its first member's login)", pr.Group)
		}
		out.PrincipalType, out.PrincipalID = "group", g.ID
	case pr.User != "":
		u, err := e.ak.FindUser(ctx, pr.User)
		if err != nil {
			return out, errors.Wrapf(err, "cannot look up user %s", pr.User)
		}
		if u == nil {
			return out, errors.Errorf("user %s doesn't exist (yet)", pr.User)
		}
		out.PrincipalType, out.PrincipalID = "user", u.ID
	case pr.ServiceAccountRef != nil:
		id, _, err := serviceAccount(ctx, e.kube, cr.GetNamespace(), *pr.ServiceAccountRef)
		if err != nil {
			return out, err
		}
		out.PrincipalType, out.PrincipalID = "service_account", id
	default:
		return out, errors.New("no principal")
	}
	return out, nil
}

// Observe reads the permission by ID or, without one, finds the principal's
// permission on the repository and adopts it. It is up to date when
// principal, repository and actions (in any order) match the spec. While cr
// is being deleted, only existence is checked, so that a deletion doesn't
// wait for principal or repository.
func (e *permissionClient) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, err := asPermission(mg)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	if meta.WasDeleted(cr) {
		id := serverID(cr)
		if id == "" {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		_, err := e.ak.GetPermission(ctx, id)
		if ak.IsNotFound(err) {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{ResourceExists: err == nil, ResourceUpToDate: true}, err
	}
	want, err := e.desired(ctx, cr)
	if err != nil {
		return managed.ExternalObservation{}, err
	}
	var have *ak.Permission
	adopted := false
	if id := serverID(cr); id != "" {
		have, err = e.ak.GetPermission(ctx, id)
		if ak.IsNotFound(err) {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		if err != nil {
			return managed.ExternalObservation{}, err
		}
	} else {
		have, err = e.ak.FindPermission(ctx, want.PrincipalType, want.PrincipalID, want.TargetType, want.TargetID)
		if err != nil {
			return managed.ExternalObservation{}, err
		}
		if have == nil {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		// Adopted: the reconciler saves the object (and so this external
		// name) only after an observation that reports a late
		// initialization; without it the ID would be lost.
		meta.SetExternalName(cr, have.ID)
		adopted = true
	}
	cr.Status.AtProvider = v1alpha1.PermissionObservation{PrincipalType: have.PrincipalType, PrincipalID: have.PrincipalID, RepositoryID: have.TargetID}
	cr.SetConditions(xpv2.Available())
	upToDate := have.PrincipalType == want.PrincipalType && have.PrincipalID == want.PrincipalID &&
		have.TargetType == want.TargetType && have.TargetID == want.TargetID && sameSet(have.Actions, want.Actions)
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: upToDate, ResourceLateInitialized: adopted}, nil
}

// Create grants the permission and records its ID as the external name.
func (e *permissionClient) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, err := asPermission(mg)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	want, err := e.desired(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}
	p, err := e.ak.CreatePermission(ctx, want)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "cannot create permission")
	}
	meta.SetExternalName(cr, p.ID)
	return managed.ExternalCreation{}, nil
}

// Update replaces the permission with the one cr asks for.
func (e *permissionClient) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, err := asPermission(mg)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	want, err := e.desired(ctx, cr)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}
	return managed.ExternalUpdate{}, errors.Wrap(e.ak.UpdatePermission(ctx, serverID(cr), want), "cannot update permission")
}

// Delete deletes the permission; without an ID, or if it is already gone,
// there is nothing to do.
func (e *permissionClient) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, err := asPermission(mg)
	if err != nil {
		return managed.ExternalDelete{}, err
	}
	cr.SetConditions(xpv2.Deleting())
	id := serverID(cr)
	if id == "" {
		return managed.ExternalDelete{}, nil
	}
	if err := e.ak.DeletePermission(ctx, id); err != nil && !ak.IsNotFound(err) {
		return managed.ExternalDelete{}, errors.Wrap(err, "cannot delete permission")
	}
	return managed.ExternalDelete{}, nil
}

// Disconnect does nothing: the client holds no connection.
func (e *permissionClient) Disconnect(context.Context) error { return nil }
