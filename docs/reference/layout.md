# Repository layout

> Reference · [Documentation](../../README.md#documentation)

```
apis/v1beta1       ProviderConfig, ClusterProviderConfig, ProviderConfigUsage
apis/v1alpha1      the managed resources (types by hand; zz_* generated)
internal/clients/ak  the REST client (only the calls used)
internal/clients   ProviderConfig -> client
internal/controller  one external client per kind, setup (safe-start gate)
cmd/provider       main
package/           crossplane.yaml, crds/ (generated)
.tekton/build.yaml CI (Pipelines-as-Code on cluster lab)
```
