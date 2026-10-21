// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package v1alpha1

// LocalReference names an object of the same namespace.
type LocalReference struct {
	// The object's name.
	Name string `json:"name"`
}

// SecretKeySelector selects a key of a Secret in the same namespace.
type SecretKeySelector struct {
	// The Secret's name.
	Name string `json:"name"`
	// The key whose value is used.
	Key string `json:"key"`
}
