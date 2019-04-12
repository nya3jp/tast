// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package build

import (
	"chromiumos/cmd/tast/logging"
)

// Config describes a configuration for building an executable package.
type Config struct {
	// Logger is used to log informational messages.
	Logger logging.Logger
	// Arch is the userland architecture to build for. It is usually given by "uname -m", but it can be different
	// if the kernel and the userland use different architectures (e.g. aarch64 kernel with armv7l userland).
	Arch string
	// Workspaces contains paths to Go workspaces (i.e. with "src" subdirectories) containing source code to be compiled.
	// These are placed into the GOPATH environment variable in the listed order.
	Workspaces []string
	// PortagePkg is the Portage package that contains the executable, including a version suffix (typically "-9999").
	// If non-empty, Build checks that the package's direct dependencies are installed and up-to-date in the host sysroot before building it.
	PortagePkg string
	// InstallPortageDeps controls whether outdated or missing Portage deps are automatically installed.
	// If false, a message is generated with the commands(s) that should be manually run and an error is returned.
	InstallPortageDeps bool
	// CheckDepsCachePath is the path to a JSON file storing a map from package names (see PortagePkg) to time.Time
	// values. Each timestamp is the latest last-modified time across Portage's overlays and database file, written
	// when the package's dependencies were last verified as being up-to-date.
	CheckDepsCachePath string
}
