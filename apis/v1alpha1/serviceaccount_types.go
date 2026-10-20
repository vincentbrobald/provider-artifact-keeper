// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceAccountParameters: a non-human user (e.g. a team's CI). Artifact
// Keeper names it svc-<name>; the external name is its ID, found by user
// name when empty.
type ServiceAccountParameters struct {
	// Without the svc- prefix Artifact Keeper adds. Can't change.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name can't change"
	Name string `json:"name"`

	// +optional
	Description string `json:"description,omitempty"`

	// Default true.
	// +optional
	Active *bool `json:"active,omitempty"`
}

// ServiceAccountObservation: what Artifact Keeper reports.
type ServiceAccountObservation struct {
	ID       string `json:"id,omitempty"`
	Username string `json:"username,omitempty"`
}

// ServiceAccountSpec defines the desired state of a ServiceAccount.
type ServiceAccountSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              ServiceAccountParameters `json:"forProvider"`
}

// ServiceAccountStatus represents the observed state of a ServiceAccount.
type ServiceAccountStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 ServiceAccountObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="USER",type="string",JSONPath=".status.atProvider.username"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,artifactkeeper}

// A ServiceAccount is an Artifact Keeper service account. Permissions name
// it (principal.serviceAccountRef); Tokens authenticate as it.
type ServiceAccount struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceAccountSpec   `json:"spec"`
	Status ServiceAccountStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceAccountList contains a list of ServiceAccount.
type ServiceAccountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceAccount `json:"items"`
}
