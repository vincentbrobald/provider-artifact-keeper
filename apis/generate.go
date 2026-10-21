// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

//go:build generate

//go:generate rm -rf ../package/crds
//go:generate go run -tags generate sigs.k8s.io/controller-tools/cmd/controller-gen object:headerFile=../hack/boilerplate.go.txt paths=./... crd:crdVersions=v1 output:artifacts:config=../package/crds
//go:generate go run -tags generate github.com/crossplane/crossplane-tools/cmd/angryjet generate-methodsets --header-file=../hack/boilerplate.go.txt ./...

package apis

import (
	_ "github.com/crossplane/crossplane-tools/cmd/angryjet" //nolint:typecheck
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"     //nolint:typecheck
)
