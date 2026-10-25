# provider-artifact-keeper: Crossplane provider for Artifact Keeper, written
# by hand against its REST API (no Terraform, no Upjet).
#
# Needs Go; `make podman-<target>` runs <target> in the pinned Go image
# instead (e.g. make podman-generate).
#
#   make generate      deepcopy, crossplane-runtime methods, CRDs (package/crds)
#   make test          unit tests; fails below COVERAGE_MIN % coverage of internal/
#   make build         bin/provider
#   make licenses      bin/licenses: the licenses of everything in bin/provider
#                      (go-licenses: texts, and the source of MPL modules) and ours
#   make package-yaml  bin/package.yaml: crossplane.yaml and the CRDs (the
#                      package's contents; the CI's image build adds it)
#   make image         the runtime image (podman), .build/image.tar

GO_IMAGE ?= docker.io/library/golang:1.26.10
VERSION  ?= $(shell cat VERSION)
GO_LICENSES_VERSION ?= v2.0.1
COVERAGE_MIN ?= 90

.PHONY: generate test build licenses package-yaml image podman-%

generate:
	cd apis && go generate -tags generate ./...

# Coverage counts internal/ only: apis/ is mostly generated, cmd/ is wiring.
test:
	@mkdir -p .build
	go test -coverprofile=.build/cover.out ./...
	@grep -E '^mode:|/internal/' .build/cover.out > .build/cover-internal.out
	@go tool cover -func=.build/cover-internal.out | awk -v min=$(COVERAGE_MIN) \
	  '/^total:/ { sub("%", "", $$3); print "coverage of internal/: " $$3 "% (minimum " min "%)"; exit ($$3 + 0 < min) }'

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X $(shell go list -m)/internal/version.Version=$(VERSION)" \
	  -o bin/provider ./cmd/provider

# Our own module is left to the copy of our files below: go-licenses would
# copy its source (MPL), bin/ included.
licenses:
	rm -rf bin/licenses
	go run github.com/google/go-licenses/v2@$(GO_LICENSES_VERSION) save ./cmd/provider \
	  --ignore $(shell go list -m) --save_path=bin/licenses
	mkdir -p bin/licenses/provider-artifact-keeper
	cp LICENSE bin/licenses/provider-artifact-keeper/

package-yaml:
	@mkdir -p bin
	{ cat package/crossplane.yaml; for f in package/crds/*.yaml; do echo '---'; cat "$$f"; done; } > bin/package.yaml

image:
	@mkdir -p .build
	podman build --target runtime -t provider-artifact-keeper:$(VERSION) --build-arg VERSION=$(VERSION) .
	podman save -q --format docker-archive -o .build/image.tar provider-artifact-keeper:$(VERSION)

podman-%:
	podman run --rm -v $(CURDIR):/src:Z -v provider-artifact-keeper-gocache:/go -w /src $(GO_IMAGE) make $*
