# provider-artifact-keeper

A Crossplane provider for [Artifact Keeper](https://github.com/artifact-keeper/artifact-keeper)
(the package registry at `artifacts.example.org`): its repositories,
permissions, OIDC login, service accounts and their tokens as Kubernetes
objects. Written by hand against Artifact Keeper's REST API (`/api/v1`,
OpenAPI at `/api/v1/openapi.json`) with crossplane-runtime v2: no
Terraform, no Upjet, no generated client.

## Documentation

The documentation follows [Diátaxis](https://diataxis.fr/): how-to guides for
tasks, reference to look things up, explanation to understand. What the
platform makes with it for each team: the gitops repository's
[What a team gets](https://github.com/vincentbrobald/gitops/blob/main/docs/reference/team-resources.md).

**How-to guides**
- [Use the provider](docs/how-to/use-the-provider.md): install, credentials, an example
- [Add a kind](docs/how-to/add-a-kind.md)

**Reference**
- [Kinds](docs/reference/kinds.md): every kind, its external name and rules
- [Build and release](docs/reference/build-and-release.md): make targets, the pipeline, the image
- [Repository layout](docs/reference/layout.md)

**Explanation**
- [Design decisions](docs/explanation/design-decisions.md): the choices, the alternatives considered, when to revisit them

## License

Mozilla Public License 2.0 (LICENSE), © 2026 Vincent Brobald; each source
file says so in its header (SPDX-License-Identifier).
