// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

// Package apis holds the provider's API types: the managed resources
// (v1alpha1) and the provider configs (v1beta1).
package apis

import (
	"k8s.io/apimachinery/pkg/runtime"

	"brobald.be/provider-artifact-keeper/apis/v1alpha1"
	"brobald.be/provider-artifact-keeper/apis/v1beta1"
)

// AddToScheme registers the types of every API version of the provider in s.
func AddToScheme(s *runtime.Scheme) error {
	if err := v1beta1.AddToScheme(s); err != nil {
		return err
	}
	return v1alpha1.AddToScheme(s)
}
