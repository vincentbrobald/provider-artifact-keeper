# Build and release

> Reference · [Documentation](../../README.md#documentation)

## Building

Go 1.26 (or `make podman-<target>`): `make generate` (controller-gen:
deepcopy and CRDs; angryjet: crossplane-runtime's resource methods),
`make test`, `make build`, `make image`.

## CI and releases

`.tekton/build.yaml` (namespace `provider-artifact-keeper` on cluster lab):
vet, tests, generated code current, `make build package-yaml`, one image
build (kaniko, Containerfile stage `package`: the binary plus
`package.yaml`, which Crossplane reads from the image), a Trivy scan
(fixable HIGH/CRITICAL fail it); on `main` skopeo pushes
`registry.example.org/providers/artifact-keeper:<VERSION>-<commit>`
and `:latest`, as the robot `robot$providers+provider-artifact-keeper-ci`
(Secret `registry-push`). Bump `VERSION` for a release worth naming.
