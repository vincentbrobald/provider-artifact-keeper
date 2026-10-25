// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package controller

import (
	"context"
	"regexp"
	"slices"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"brobald.be/provider-artifact-keeper/apis/v1alpha1"
	"brobald.be/provider-artifact-keeper/internal/clients/ak"
)

// repositoryKey returns the key of a repository given either by ref (a
// Repository object of namespace ns, whose external name is the key) or by
// key itself; ref wins when both are set. It fails if neither is set or if
// the object has no external name yet.
func repositoryKey(ctx context.Context, kube client.Client, ns string, ref *v1alpha1.LocalReference, key string) (string, error) {
	if ref == nil {
		if key == "" {
			return "", errors.New("neither a repository reference nor a key")
		}
		return key, nil
	}
	r := &v1alpha1.Repository{}
	if err := kube.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, r); err != nil {
		return "", errors.Wrapf(err, "cannot get Repository %s", ref.Name)
	}
	if k := meta.GetExternalName(r); k != "" {
		return k, nil
	}
	return "", errors.Errorf("Repository %s has no key yet", ref.Name)
}

// repositoryID returns Artifact Keeper's ID of the repository given by ref
// or key (see repositoryKey); the repository must exist.
func repositoryID(ctx context.Context, kube client.Client, c *ak.Client, ns string, ref *v1alpha1.LocalReference, key string) (string, error) {
	k, err := repositoryKey(ctx, kube, ns, ref, key)
	if err != nil {
		return "", err
	}
	r, err := c.GetRepository(ctx, k)
	if err != nil {
		return "", errors.Wrapf(err, "repository %s", k)
	}
	return r.ID, nil
}

// serviceAccount returns the ID and the user name of the ServiceAccount
// object ref names in namespace ns. It fails if the object is missing or has
// no ID yet; the user name is empty until the object's first observation.
func serviceAccount(ctx context.Context, kube client.Client, ns string, ref v1alpha1.LocalReference) (id, username string, err error) {
	sa := &v1alpha1.ServiceAccount{}
	if err := kube.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, sa); err != nil {
		return "", "", errors.Wrapf(err, "cannot get ServiceAccount %s", ref.Name)
	}
	id = serverID(sa)
	if id == "" {
		return "", "", errors.Errorf("ServiceAccount %s has no ID yet", ref.Name)
	}
	return id, sa.Status.AtProvider.Username, nil
}

// sameSet reports whether a and b hold the same strings, in any order
// (duplicates count). Neither is modified.
func sameSet(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}

// as returns mg as a *T, or an error if it is another kind.
func as[T any, PT interface {
	*T
	resource.Managed
}](mg resource.Managed) (PT, error) {
	cr, ok := mg.(PT)
	if !ok {
		var t T
		return nil, errors.Errorf("not a %T", t)
	}
	return cr, nil
}

// getOrAdopt returns the external object by cr's ID or, without one, the one
// find returns, whose ID it then records as cr's external name (adopted). A
// nil object means there is none; an object gone by ID is no error.
func getOrAdopt[T any](ctx context.Context, cr metav1.Object, get func(context.Context, string) (*T, error), find func() (*T, error), idOf func(*T) string) (have *T, adopted bool, err error) {
	if id := serverID(cr); id != "" {
		have, err = get(ctx, id)
		if ak.IsNotFound(err) {
			return nil, false, nil
		}
		return have, false, err
	}
	if have, err = find(); err != nil || have == nil {
		return nil, false, err
	}
	// The reconciler saves the object (and so this external name) only after
	// an observation that reports a late initialization: the caller must
	// report adopted as one, or the ID is lost.
	meta.SetExternalName(cr, idOf(have))
	return have, true, nil
}

// deref returns *p, or def when p is nil.
func deref[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

// uuidRE matches a lower-case UUID, the form of Artifact Keeper's IDs.
var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// serverID returns o's external name if it is an Artifact Keeper ID (a
// UUID), else "": any other external name, such as the object's name, counts
// as none.
func serverID(o metav1.Object) string {
	if id := meta.GetExternalName(o); uuidRE.MatchString(id) {
		return id
	}
	return ""
}
