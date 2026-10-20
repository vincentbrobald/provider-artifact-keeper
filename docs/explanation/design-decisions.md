# Design decisions

> Explanation · [Documentation](../../README.md#documentation)

The choices provider-artifact-keeper is built on, what else was considered,
and when to look at them again. Why Artifact Keeper and why its content comes
from Git: the gitops repository's [Design decisions](https://github.com/vincentbrobald/gitops/blob/main/docs/explanation/design-decisions.md).

### API groups under the owner's public domain

**Decision.** The kinds live in `artifactkeeper.crossplane.brobald.be`, and the Go module is
`brobald.be/provider-artifact-keeper`.
**Why.** A group must be a domain its owner controls; the site's internal
domain was used at first, and it must not appear in what is published
(a sanitized copy of the lab is shared publicly). `brobald.be` is public and
the owner's.
The package moved with it, to `docker.io/vincentbrobald/provider-artifact-keeper`:
Crossplane refuses two packages from the same source, and the old and new
versions run side by side during the migration.
**Cost.** Renaming a group creates new kinds: the objects of the old group
are migrated to the new one on the cluster (gitops).
**Revisit if** the provider moves to a shared home (e.g. crossplane-contrib),
whose group it would then take.

### Written by hand on crossplane-runtime v2

**Decision.** Types, REST client and external clients are written by hand
against Artifact Keeper's REST API; only deepcopy, CRDs and crossplane-runtime's
resource methods are generated.
**Why.** No Terraform provider exists for Upjet to build on, and no
Crossplane provider for Artifact Keeper existed.
**Alternatives.** A generic HTTP provider (requests described in YAML): no
real diffing, no handling of values shown only once (tokens), awkward
references. A client generated from the OpenAPI document: large, while five
kinds use a few calls.
**Revisit if** Artifact Keeper publishes a Terraform provider (Upjet would
then cover more of its API for less code).

### Only the calls used

**Decision.** `internal/clients/ak` implements the endpoints the five kinds
need and nothing else.
**Why.** Less code to review and keep in step with the server's API.

### Adoption by natural key

**Decision.** Objects without an external name are looked up by what
identifies them (a repository's key, an OIDC provider's name, a service
account's `svc-<name>`, a permission's principal and repository) and adopted.
**Why.** Objects made by hand or before the provider existed are taken over
instead of duplicated.

### Tokens are never adopted

**Decision.** A token of the same name without our external name is revoked
and made anew; a change of scopes or repositories, or expiry, revokes it and
a new one is made.
**Why.** Artifact Keeper shows a token's value only when it is made: an
adopted token's value would be unknown, and the connection Secret empty.

### Groups come from the OIDC login, never pre-created

**Decision.** Permissions name OIDC groups that Artifact Keeper creates at a
member's first login; until then they wait (Synced False, retried).
**Why.** Artifact Keeper refuses OIDC memberships for a group of the same name
that came from another source, so pre-creating it locally would lock the
team out.

### An API token, never a password

**Decision.** The ProviderConfig holds an admin's API token.
**Why.** Artifact Keeper rate-limits password logins for minutes; a
reconciling controller would lock itself out.

### OIDC client secret tracked by hash

**Decision.** `status.atProvider.clientSecretHash` records the hash of the
secret last sent.
**Why.** The server never returns the secret, so a changed Secret is detected
by its hash instead.
