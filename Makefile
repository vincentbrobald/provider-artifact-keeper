# provider-artifact-keeper: Crossplane provider for Artifact Keeper, written
# by hand against its REST API (no Terraform, no Upjet).
#
# Needs Go; `make podman-<target>` runs <target> in the pinned Go image
# instead (e.g. make podman-generate).
#
#   make generate      deepcopy, crossplane-runtime methods, CRDs (package/crds)
#   make test          unit tests
#   make build         bin/provider
#   make package-yaml  bin/package.yaml: crossplane.yaml and the CRDs (the
#                      package's contents; the CI's image build adds it)
#   make image         the runtime image (podman), .build/image.tar

GO_IMAGE ?= docker.io/library/golang:1.26.9
VERSION  ?= $(shell cat VERSION)

.PHONY: generate test build package-yaml image podman-%

generate:
	cd apis && go generate -tags generate ./...

test:
	go test ./...

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X $(shell go list -m)/internal/version.Version=$(VERSION)" \
	  -o bin/provider ./cmd/provider

package-yaml:
	@mkdir -p bin
	{ cat package/crossplane.yaml; for f in package/crds/*.yaml; do echo '---'; cat "$$f"; done; } > bin/package.yaml

image:
	@mkdir -p .build
	podman build --target runtime -t provider-artifact-keeper:$(VERSION) --build-arg VERSION=$(VERSION) .
	podman save -q --format docker-archive -o .build/image.tar provider-artifact-keeper:$(VERSION)

podman-%:
	podman run --rm -v $(CURDIR):/src:Z -v provider-artifact-keeper-gocache:/go -w /src $(GO_IMAGE) make $*
