// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TokenParameters: an API token of a service account, published in the
// connection Secret (key token; user name in key username). Artifact Keeper
// shows a token only when it is made: a change of the parameters makes a
// new token (and revokes the old one). The external name is its ID, found
// by name when empty.
type TokenParameters struct {
	// The ServiceAccount object (this namespace) the token belongs to.
	ServiceAccountRef LocalReference `json:"serviceAccountRef"`

	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Scopes, e.g. read:artifacts, write:artifacts.
	// +kubebuilder:validation:MinItems=1
	Scopes []string `json:"scopes"`

	// Restricts the token to these repositories (Repository objects of this
	// namespace, or keys); every repository the account may use when both
	// are empty.
	// +optional
	RepositoryRefs []LocalReference `json:"repositoryRefs,omitempty"`
	// +optional
	RepositoryKeys []string `json:"repositoryKeys,omitempty"`

	// Days until the token expires; Artifact Keeper's policy when unset.
	// +optional
	ExpiresInDays *int64 `json:"expiresInDays,omitempty"`
}

// TokenObservation: what Artifact Keeper reports.
type TokenObservation struct {
	ID               string   `json:"id,omitempty"`
	ServiceAccountID string   `json:"serviceAccountId,omitempty"`
	Prefix           string   `json:"prefix,omitempty"`
	ExpiresAt        string   `json:"expiresAt,omitempty"`
	RepositoryIDs    []string `json:"repositoryIds,omitempty"`
	Scopes           []string `json:"scopes,omitempty"`
}

// TokenSpec defines the desired state of a Token.
type TokenSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              TokenParameters `json:"forProvider"`
}

// TokenStatus represents the observed state of a Token.
type TokenStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 TokenObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="PREFIX",type="string",JSONPath=".status.atProvider.prefix"
// +kubebuilder:printcolumn:name="EXPIRES",type="string",JSONPath=".status.atProvider.expiresAt"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,artifactkeeper}

// A Token is an API token of a ServiceAccount.
type Token struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TokenSpec   `json:"spec"`
	Status TokenStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TokenList contains a list of Token.
type TokenList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Token `json:"items"`
}
