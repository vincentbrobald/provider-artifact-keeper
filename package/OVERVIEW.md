# provider-artifact-keeper

A [Crossplane](https://www.crossplane.io/) provider for
[Artifact Keeper](https://github.com/artifact-keeper/artifact-keeper), the
package registry: its repositories (Maven, npm, PyPI, generic and more;
local, remote and virtual), permissions, OIDC login, service accounts and
their tokens, as Kubernetes objects.

It is written by hand against Artifact Keeper's REST API (`/api/v1`) with
crossplane-runtime v2: no Terraform, no Upjet, no generated client.

## What this image is

The Crossplane package itself: Crossplane reads the package metadata and
CRDs (`/package.yaml`) from it and runs the same image as the provider. The
provider binary is `/usr/local/bin/provider`, on distroless static, as a
non-root user. The licenses of everything in it are under `/licenses`.

## Install

Crossplane v2:

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata: {name: artifact-keeper}
spec:
  package: docker.io/vincentbrobald/provider-artifact-keeper:<tag>
---
apiVersion: artifactkeeper.crossplane.brobald.be/v1beta1
kind: ClusterProviderConfig
metadata: {name: default}
spec:
  credentials:
    source: Secret
    secretRef: {namespace: crossplane-system, name: artifact-keeper-credentials, key: credentials}
```

The Secret's key holds `{"url": "https://artifacts.example.org", "token": "..."}`:
an API token of an admin (the provider never uses a password). The provider
supports safe-start: activate `*.artifactkeeper.crossplane.brobald.be` with a
ManagedResourceActivationPolicy. Details and an example (a team's repository,
its group's permission and a CI token):
[Use the provider](https://github.com/vincentbrobald/provider-artifact-keeper/blob/main/docs/how-to/use-the-provider.md).

## Kinds

All namespaced, in `artifactkeeper.crossplane.brobald.be/v1alpha1`:

| Kind | What |
|---|---|
| `Repository` | a repository: format, type (local, remote, virtual and its members), visibility, quota |
| `Permission` | read, write, delete or admin on a repository, for a group, a user or a service account |
| `OIDCProvider` | login through an OIDC issuer, its groups mapped to Artifact Keeper groups |
| `ServiceAccount` | a non-human account |
| `Token` | a service account's API token, restricted to scopes and repositories, written to a Secret |

External names, adoption and caveats:
[Kinds](https://github.com/vincentbrobald/provider-artifact-keeper/blob/main/docs/reference/kinds.md).

## Tags

- `v<version>-<commit>`: one per commit of the main branch (e.g. `v0.2.0-4c866bc`);
  pin one of these.
- `latest`: the newest of them.

## Source and license

Source: [github.com/vincentbrobald/provider-artifact-keeper](https://github.com/vincentbrobald/provider-artifact-keeper).
Mozilla Public License 2.0. `/licenses` in the image holds it and every
dependency's license.
