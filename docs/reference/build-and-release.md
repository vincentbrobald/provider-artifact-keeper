# Build and release

> Reference · [Documentation](../../README.md#documentation)

## Building

Go 1.26 (or `make podman-<target>`): `make generate` (controller-gen:
deepcopy and CRDs; angryjet: crossplane-runtime's resource methods),
`make test`, `make build`, `make licenses` (`bin/licenses`), `make image`.

## CI and releases

`.tekton/build.yaml` (namespace `provider-artifact-keeper` on cluster lab):
vet, tests, generated code current, `make build licenses package-yaml`, one image
build (kaniko, Containerfile stage `package`: the binary plus
`package.yaml`, which Crossplane reads from the image), a Trivy scan
(fixable HIGH/CRITICAL fail it); on `main` skopeo pushes
`<registry>/providers/artifact-keeper:<VERSION>-<commit>` and `:latest` to
the site's Harbor (`<registry>`: key `registry` of Secret `registry-push`),
as the robot `robot$providers+provider-artifact-keeper-ci`. Then, unless the image's config or a layer names
the site (its domain, for one), the same tags go to Docker Hub,
`docker.io/vincentbrobald/provider-artifact-keeper`: the package the
published copies of the lab use (Secret `dockerhub-push`, keys `username`,
`token`, same namespace). Bump `VERSION` for a release worth naming.

Every image carries the licenses of what it contains under `/licenses`
(`make licenses`: go-licenses copies each dependency's license, and the
source of the MPL-2.0 ones; plus this repository's LICENSE), and labels
`org.opencontainers.image.source` (the public repository) and
`org.opencontainers.image.licenses`.
