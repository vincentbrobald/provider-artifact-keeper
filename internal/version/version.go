// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

// Package version is the provider's version, set at build time
// (-ldflags -X .../internal/version.Version=...).
package version

// Version is the provider's version; "dev" when the build doesn't set it.
var Version = "dev"
