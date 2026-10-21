// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Artifact Keeper names it svc-<name>. The external name is its ID; without
// one, svc-<name> is adopted if it exists.
type ServiceAccountParameters struct {
	// Without the svc- prefix Artifact Keeper adds. Can't change.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name can't change"
	Name string `json:"name"`

	// Free text, set at creation only.
	// +optional
	Description string `json:"description,omitempty"`

	// Default true.
	// +optional
	Active *bool `json:"active,omitempty"`
}

type ServiceAccountObservation struct {
	// The account's ID (also the external name).
	ID string `json:"id,omitempty"`
	// Its user name, svc-<name>; Permissions and Tokens refer to it.
	Username string `json:"username,omitempty"`
}

type ServiceAccountSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              ServiceAccountParameters `json:"forProvider"`
}

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

// A ServiceAccount is an Artifact Keeper service account.
type ServiceAccount struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceAccountSpec   `json:"spec"`
	Status ServiceAccountStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type ServiceAccountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceAccount `json:"items"`
}
