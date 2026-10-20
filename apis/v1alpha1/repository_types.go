// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RepositoryParameters: what Artifact Keeper's repository should be. Its key
// (the path segment clients use: /maven/<key>, /npm/<key>, ...) is the
// external name, by default the object's name.
type RepositoryParameters struct {
	// Display name; the key when empty.
	// +optional
	Name string `json:"name,omitempty"`

	// +optional
	Description string `json:"description,omitempty"`

	// Package format: maven, npm, pypi, generic, ... (Artifact Keeper's
	// names). Can't change.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="format can't change"
	Format string `json:"format"`

	// local (hosted), remote (pull-through cache of upstreamURL) or virtual
	// (the members, in order). Can't change.
	// +kubebuilder:validation:Enum=local;remote;virtual
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="type can't change"
	Type string `json:"type"`

	// The upstream registry of a remote repository. Can't change (the API
	// has no update for it).
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="upstreamURL can't change"
	UpstreamURL string `json:"upstreamURL,omitempty"`

	// Anonymous read access (allow_anonymous_access). Default false.
	// +optional
	Public *bool `json:"public,omitempty"`

	// Storage quota in bytes; none when unset.
	// +optional
	QuotaBytes *int64 `json:"quotaBytes,omitempty"`

	// Members of a virtual repository, first first: a Repository object of
	// this namespace (repositoryRef) or any repository's key.
	// +optional
	Members []RepositoryMember `json:"members,omitempty"`
}

// RepositoryMember is one member of a virtual repository.
// +kubebuilder:validation:XValidation:rule="has(self.repositoryRef) != has(self.key)",message="exactly one of repositoryRef and key"
type RepositoryMember struct {
	// +optional
	RepositoryRef *LocalReference `json:"repositoryRef,omitempty"`
	// +optional
	Key string `json:"key,omitempty"`
}

// RepositoryObservation: what Artifact Keeper reports.
type RepositoryObservation struct {
	ID               string   `json:"id,omitempty"`
	Format           string   `json:"format,omitempty"`
	Type             string   `json:"type,omitempty"`
	StorageUsedBytes int64    `json:"storageUsedBytes,omitempty"`
	Members          []string `json:"members,omitempty"`
}

// RepositorySpec defines the desired state of a Repository.
type RepositorySpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              RepositoryParameters `json:"forProvider"`
}

// RepositoryStatus represents the observed state of a Repository.
type RepositoryStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 RepositoryObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="KEY",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="FORMAT",type="string",JSONPath=".spec.forProvider.format"
// +kubebuilder:printcolumn:name="TYPE",type="string",JSONPath=".spec.forProvider.type"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,artifactkeeper}

// A Repository is an Artifact Keeper repository. Deleting it deletes the
// repository and its packages: leave Delete out of managementPolicies for
// one that matters.
type Repository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RepositorySpec   `json:"spec"`
	Status RepositoryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RepositoryList contains a list of Repository.
type RepositoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Repository `json:"items"`
}
