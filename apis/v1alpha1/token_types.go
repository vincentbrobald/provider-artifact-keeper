// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TokenParameters are an API token of a service account, published in the
// connection Secret (key token; the account's user name in key username).
// The external name is the token's ID. Artifact Keeper shows a token's value
// only when it is made, so a token is never adopted: without an external
// name, a token of the same name is revoked and a new one made. A change of
// scopes or repositories, or the token's expiry, revokes it and makes a new
// one; a change of name or expiresInDays applies only to the next token.
type TokenParameters struct {
	// The ServiceAccount object (this namespace) the token belongs to.
	ServiceAccountRef LocalReference `json:"serviceAccountRef"`

	// The token's name in Artifact Keeper, unique per service account.
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
	// Keys of repositories to restrict the token to (any repository,
	// including ones no Repository object manages).
	// +optional
	RepositoryKeys []string `json:"repositoryKeys,omitempty"`

	// Days until the token expires; Artifact Keeper's policy when unset.
	// +optional
	ExpiresInDays *int64 `json:"expiresInDays,omitempty"`
}

// TokenObservation is what Artifact Keeper reports about the token (never
// its value).
type TokenObservation struct {
	// The token's ID (also the external name).
	ID string `json:"id,omitempty"`
	// The ID of the service account it belongs to, kept so that the token
	// can be revoked after the ServiceAccount object is gone.
	ServiceAccountID string `json:"serviceAccountId,omitempty"`
	// The token's first characters, to recognise it.
	Prefix string `json:"prefix,omitempty"`
	// When the token expires; empty if it doesn't.
	ExpiresAt string `json:"expiresAt,omitempty"`
	// The IDs of the repositories it is restricted to.
	RepositoryIDs []string `json:"repositoryIds,omitempty"`
	// Its scopes.
	Scopes []string `json:"scopes,omitempty"`
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
