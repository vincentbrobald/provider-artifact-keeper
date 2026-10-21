// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Group and Version are the API group and version of the package's types.
const (
	Group   = "artifactkeeper.crossplane.brobald.be"
	Version = "v1alpha1"
)

var (
	// SchemeGroupVersion is the group and version the managed resource kinds are
	// registered under.
	SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: Version}
	// SchemeBuilder registers the package's types.
	SchemeBuilder = runtime.NewSchemeBuilder(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion,
			&Repository{}, &RepositoryList{},
			&Permission{}, &PermissionList{},
			&OIDCProvider{}, &OIDCProviderList{},
			&ServiceAccount{}, &ServiceAccountList{},
			&Token{}, &TokenList{},
		)
		metav1.AddToGroupVersion(s, SchemeGroupVersion)
		return nil
	})
	// AddToScheme adds the package's types to a scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

// gvk returns the GroupVersionKind of o, a value (not a pointer) of one of
// the package's types: its kind is the Go type's name.
func gvk(o any) schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind(reflect.TypeOf(o).Name())
}

// The GroupVersionKinds of the package's types.
var (
	RepositoryGroupVersionKind     = gvk(Repository{})
	PermissionGroupVersionKind     = gvk(Permission{})
	OIDCProviderGroupVersionKind   = gvk(OIDCProvider{})
	ServiceAccountGroupVersionKind = gvk(ServiceAccount{})
	TokenGroupVersionKind          = gvk(Token{})
)
