// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package v1alpha1

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PermissionParameters are who may do what on a repository. Artifact Keeper
// allows one permission per principal and repository: the external name is
// the permission's ID, and without one the permission of the same principal
// on the same repository is adopted.
// +kubebuilder:validation:XValidation:rule="has(self.repositoryRef) != has(self.repositoryKey)",message="exactly one of repositoryRef and repositoryKey"
type PermissionParameters struct {
	// The principal: exactly one of group, user, serviceAccountRef.
	Principal PermissionPrincipal `json:"principal"`

	// The repository: exactly one of repositoryRef and repositoryKey.
	// A Repository object of this namespace.
	// +optional
	RepositoryRef *LocalReference `json:"repositoryRef,omitempty"`
	// A repository's key (any repository, including ones no Repository
	// object manages).
	// +optional
	RepositoryKey string `json:"repositoryKey,omitempty"`

	// Actions: read, write, delete, admin (Artifact Keeper's names).
	// +kubebuilder:validation:MinItems=1
	Actions []string `json:"actions"`
}

// PermissionPrincipal names who the permission is for.
// +kubebuilder:validation:XValidation:rule="(has(self.group) ? 1 : 0) + (has(self.user) ? 1 : 0) + (has(self.serviceAccountRef) ? 1 : 0) == 1",message="exactly one of group, user, serviceAccountRef"
type PermissionPrincipal struct {
	// A group by name. A group mirrored from the OIDC login (e.g.
	// team-<team>) exists once one of its members has signed in: until then
	// the permission waits (Synced False).
	// +optional
	Group string `json:"group,omitempty"`
	// A user by name.
	// +optional
	User string `json:"user,omitempty"`
	// A ServiceAccount object of this namespace.
	// +optional
	ServiceAccountRef *LocalReference `json:"serviceAccountRef,omitempty"`
}

// PermissionObservation is what Artifact Keeper reports about the
// permission.
type PermissionObservation struct {
	// group, user or service_account.
	PrincipalType string `json:"principalType,omitempty"`
	// The principal's ID.
	PrincipalID string `json:"principalId,omitempty"`
	// The repository's ID.
	RepositoryID string `json:"repositoryId,omitempty"`
}

// PermissionSpec defines the desired state of a Permission.
type PermissionSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              PermissionParameters `json:"forProvider"`
}

// PermissionStatus represents the observed state of a Permission.
type PermissionStatus struct {
	xpv2.ManagedResourceStatus `json:",inline"`
	AtProvider                 PermissionObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,artifactkeeper}

// A Permission grants a group, user or service account actions on a
// repository.
type Permission struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PermissionSpec   `json:"spec"`
	Status PermissionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PermissionList contains a list of Permission.
type PermissionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Permission `json:"items"`
}
