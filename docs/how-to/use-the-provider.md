# Use the provider

> How-to · [Documentation](../../README.md#documentation)

Install the provider in a cluster, give it Artifact Keeper credentials, and
manage objects with it. Every kind: [Kinds](../reference/kinds.md).

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata: {name: provider-artifact-keeper}
spec:
  package: docker.io/vincentbrobald/provider-artifact-keeper:<VERSION>-<commit>
---
apiVersion: artifactkeeper.crossplane.brobald.be/v1beta1
kind: ClusterProviderConfig
metadata: {name: default}
spec:
  credentials:
    source: Secret
    secretRef: {namespace: crossplane-system, name: artifact-keeper-credentials, key: credentials}
```

The Secret holds JSON under that key (`{"url": "https://artifacts.example", "token": "..."}`)
or, when the key is missing, the flat keys `url` and `token`. The token is
an API token of an admin (`POST /api/v1/users/me/tokens`): Artifact Keeper
rate-limits password logins, so the provider never uses a password. TLS
trust is the system's: add a private CA with a `DeploymentRuntimeConfig`
mounting it and setting `SSL_CERT_DIR`.

With a ManagedResourceActivationPolicy, activate
`*.artifactkeeper.crossplane.brobald.be` (the provider supports
safe-start).

Example: a team's Maven repository, write access for its OIDC group, and a
CI token restricted to it:

```yaml
apiVersion: artifactkeeper.crossplane.brobald.be/v1alpha1
kind: Repository
metadata: {name: bobteam-maven, namespace: team-bobteam}
spec:
  managementPolicies: [Observe, Create, Update]   # never deleted by Crossplane
  providerConfigRef: {kind: ClusterProviderConfig, name: default}
  forProvider: {format: maven, type: local}
---
apiVersion: artifactkeeper.crossplane.brobald.be/v1alpha1
kind: Permission
metadata: {name: bobteam-maven-team, namespace: team-bobteam}
spec:
  providerConfigRef: {kind: ClusterProviderConfig, name: default}
  forProvider:
    principal: {group: team-bobteam}
    repositoryRef: {name: bobteam-maven}
    actions: [read, write]
```
