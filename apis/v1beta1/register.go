// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package v1beta1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	Group   = "artifactkeeper.crossplane.brobald.be"
	Version = "v1beta1"
)

var (
	SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: Version}
	SchemeBuilder      = runtime.NewSchemeBuilder(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion,
			&ProviderConfig{}, &ProviderConfigList{},
			&ClusterProviderConfig{}, &ClusterProviderConfigList{},
			&ProviderConfigUsage{}, &ProviderConfigUsageList{},
		)
		metav1.AddToGroupVersion(s, SchemeGroupVersion)
		return nil
	})
	AddToScheme = SchemeBuilder.AddToScheme
)

func gvk(o any) schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind(reflect.TypeOf(o).Name())
}

var (
	ProviderConfigGroupVersionKind          = gvk(ProviderConfig{})
	ProviderConfigGroupKind                 = ProviderConfigGroupVersionKind.GroupKind().String()
	ClusterProviderConfigGroupVersionKind   = gvk(ClusterProviderConfig{})
	ClusterProviderConfigGroupKind          = ClusterProviderConfigGroupVersionKind.GroupKind().String()
	ProviderConfigUsageGroupVersionKind     = gvk(ProviderConfigUsage{})
	ProviderConfigUsageListGroupVersionKind = gvk(ProviderConfigUsageList{})
)
