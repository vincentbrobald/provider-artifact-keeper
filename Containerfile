# Runtime image of provider-artifact-keeper: the provider binary on distroless static
# (no shell, non-root), embedded in the Crossplane package (crossplane xpkg
# build).
#   podman build --target runtime -t provider-artifact-keeper .  # compiles (make image)
#   kaniko ... --target package                          # CI: bin/provider from
#                    `make build` and bin/package.yaml from `make package-yaml`
# GO_BASE, BASE: the CI passes the same images through the registry's
# pull-through mirror.
ARG GO_BASE=docker.io/library/golang:1.26.9-trixie
ARG BASE=gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3

FROM ${GO_BASE} AS build
ENV GOTOOLCHAIN=local CGO_ENABLED=0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN go build -trimpath -ldflags "-s -w -X brobald.be/provider-artifact-keeper/internal/version.Version=${VERSION}" \
      -o /provider ./cmd/provider

FROM ${BASE} AS runtime
COPY --from=build /provider /usr/local/bin/provider
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/provider"]

# The CI's image: the binary its test step built (make build), so that Go
# isn't compiled a second time inside the image build.
FROM ${BASE} AS prebuilt
COPY bin/provider /usr/local/bin/provider
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/provider"]

# The CI's Crossplane package: the runtime image above plus package.yaml
# (crossplane.yaml and the CRDs, make package-yaml). Crossplane reads
# package.yaml from the image's files when no layer is annotated as the
# package (xpkg specification), and runs the package image itself as the
# provider when no other runtime image is named.
FROM prebuilt AS package
COPY bin/package.yaml /package.yaml
