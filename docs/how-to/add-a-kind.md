# Add a kind

> How-to · [Documentation](../../README.md#documentation)

Manage another Artifact Keeper object as a Crossplane kind.

Write its types in `apis/v1alpha1` (spec embedding
`xpv2.ManagedResourceSpec`, status `xpv2.ManagedResourceStatus`, the
kubebuilder markers of the others), registered in `register.go`; its
calls in `internal/clients/ak`; an external client (Observe, Create,
Update, Delete) in `internal/controller` and an entry in `kinds()`;
`make generate test`. Observe must work during deletion without
resolving references (they may be gone).
