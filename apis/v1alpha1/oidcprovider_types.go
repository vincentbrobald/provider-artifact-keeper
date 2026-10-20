// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OIDCProviderParameters: an OIDC login of Artifact Keeper. The external
// name is its ID, found by name when empty.
type OIDCProviderParameters struct {
	// The name shown on the login page; also how an existing provider is
	// found.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// +kubebuilder:validation:MinLength=1
	IssuerURL string `json:"issuerURL"`

	// +kubebuilder:validation:MinLength=1
	ClientID string `json:"clientID"`

	// The client's secret (Artifact Keeper never shows it back: a change of
	// the Secret is noticed by its hash, status.atProvider.clientSecretHash).
	ClientSecretSecretRef SecretKeySelector `json:"clientSecretSecretRef"`

	// Default openid, profile, email.
	// +optional
	Scopes []string `json:"scopes,omitempty"`

	// The URL the IdP sends users back to; Artifact Keeper derives it from
	// the request when empty (.../api/v1/auth/sso/oidc/callback).
	// +optional
	RedirectURI string `json:"redirectURI,omitempty"`

	// Claim with the user's groups. Default groups.
	// +optional
	GroupsClaim string `json:"groupsClaim,omitempty"`

	// Claim with the user name. Default preferred_username.
	// +optional
	UsernameClaim string `json:"usernameClaim,omitempty"`

	// Members of this IdP group are admins.
	// +optional
	AdminGroup string `json:"adminGroup,omitempty"`

	// Mirror the groups claim as Artifact Keeper groups (created on first
	// sight, memberships reconciled at each login). Default true.
	// +optional
	MapGroupsToGroups *bool `json:"mapGroupsToGroups,omitempty"`

	// Create users at their first login. Default true.
	// +optional
	AutoCreateUsers *bool `json:"autoCreateUsers,omitempty"`

	// Default true.
	// +optional
	PKCE *bool `json:"pkce,omitempty"`

	// Default true.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// OIDCProviderObservation: what Artifact Keeper reports.
type OIDCProviderObservation struct {
	ID string `json:"id,omitempty"`
	// SHA-256 of the client secret last sent.
	ClientSecretHash string `json:"clientSecretHash,omitempty"`
}

// OIDCProviderSpec defines the desired state of an OIDCProvider.
type OIDCProviderSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              OIDCProviderParameters `json:"forProvider"`
}

// OIDCProviderStatus represents the observed state of an OIDCProvider.
type OIDCProviderStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 OIDCProviderObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="ISSUER",type="string",JSONPath=".spec.forProvider.issuerURL"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,artifactkeeper}

// An OIDCProvider is a login of Artifact Keeper through an OIDC IdP
// (Keycloak).
type OIDCProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OIDCProviderSpec   `json:"spec"`
	Status OIDCProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OIDCProviderList contains a list of OIDCProvider.
type OIDCProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OIDCProvider `json:"items"`
}
