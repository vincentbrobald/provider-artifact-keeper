// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Group and version of the managed resources.
const (
	Group   = "artifactkeeper.crossplane.brobald.be"
	Version = "v1alpha1"
)

var (
	// SchemeGroupVersion is the group version of these kinds.
	SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: Version}
	// SchemeBuilder registers them.
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
	// AddToScheme adds them to a scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func gvk(o any) schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind(reflect.TypeOf(o).Name())
}

// The managed resources' kinds.
var (
	RepositoryGroupVersionKind     = gvk(Repository{})
	PermissionGroupVersionKind     = gvk(Permission{})
	OIDCProviderGroupVersionKind   = gvk(OIDCProvider{})
	ServiceAccountGroupVersionKind = gvk(ServiceAccount{})
	TokenGroupVersionKind          = gvk(Token{})
)
