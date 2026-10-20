# Kinds

> Reference · [Documentation](../../README.md#documentation)

Every kind the provider installs, its external name and its rules. To use
them: [Use the provider](../how-to/use-the-provider.md).

Namespaced (Crossplane v2), group `artifactkeeper.crossplane.brobald.be`,
version `v1alpha1`. References (`...Ref`) name objects of the same namespace.

| Kind | External name | Notes |
|---|---|---|
| `Repository` | its key (default: the object's name) | `format` (maven, npm, pypi, generic, ...) and `type` (local, remote, virtual) can't change, nor `upstreamURL` (remote); `public`, `quotaBytes`, `description`; a virtual one's `members` in order (`repositoryRef` or `key`). **Deleting it deletes the repository and its packages**: leave `Delete` out of `managementPolicies` for one that matters |
| `Permission` | its ID; adopted by principal and repository when unset | `principal` (`group`, `user` or `serviceAccountRef`), `repositoryRef` or `repositoryKey`, `actions` (read, write, delete, admin). A group mirrored from the OIDC login exists after its first member's login: until then the permission waits (Synced False, retried) |
| `OIDCProvider` | its ID; adopted by `name` when unset | `issuerURL`, `clientID`, `clientSecretSecretRef`, `groupsClaim`, `usernameClaim`, `redirectURI`, `adminGroup`, `mapGroupsToGroups` (default true: the groups claim becomes Artifact Keeper groups). The secret isn't readable back: its hash in `status.atProvider.clientSecretHash` tells when the Secret changed |
| `ServiceAccount` | its ID; adopted by `svc-<name>` when unset | Artifact Keeper prefixes `svc-`; `active` |
| `Token` | its ID | a token of `serviceAccountRef`: `scopes` (read:artifacts, write:artifacts, ...), restricted to `repositoryRefs`/`repositoryKeys`, `expiresInDays`. Published in the connection Secret (`writeConnectionSecretToRef`): keys `token`, `username`. Never adopted (Artifact Keeper shows a token only once): a token of the same name without the external name is revoked and made anew; a change of scopes or repositories, or expiry, revokes it and a new one is made at the next reconcile |
