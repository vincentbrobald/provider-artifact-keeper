// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package controller

import (
	"context"
	"regexp"
	"sort"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"brobald.be/provider-artifact-keeper/apis/v1alpha1"
	"brobald.be/provider-artifact-keeper/internal/clients/ak"
)

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

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func deref[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func serverID(o metav1.Object) string {
	if id := meta.GetExternalName(o); uuidRE.MatchString(id) {
		return id
	}
	return ""
}
