// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package v1beta1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Group and Version are the API group and version of the package's types.
const (
	Group   = "artifactkeeper.crossplane.brobald.be"
	Version = "v1beta1"
)

var (
	// SchemeGroupVersion is the group and version the provider config kinds
	// are registered under.
	SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: Version}
	// SchemeBuilder registers the package's types.
	SchemeBuilder = runtime.NewSchemeBuilder(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion,
			&ProviderConfig{}, &ProviderConfigList{},
			&ClusterProviderConfig{}, &ClusterProviderConfigList{},
			&ProviderConfigUsage{}, &ProviderConfigUsageList{},
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

// The GroupVersionKinds of the package's types, and the group kinds
// (Kind.group) the provider config controllers are named after.
var (
	ProviderConfigGroupVersionKind          = gvk(ProviderConfig{})
	ProviderConfigGroupKind                 = ProviderConfigGroupVersionKind.GroupKind().String()
	ClusterProviderConfigGroupVersionKind   = gvk(ClusterProviderConfig{})
	ClusterProviderConfigGroupKind          = ClusterProviderConfigGroupVersionKind.GroupKind().String()
	ProviderConfigUsageGroupVersionKind     = gvk(ProviderConfigUsage{})
	ProviderConfigUsageListGroupVersionKind = gvk(ProviderConfigUsageList{})
)
