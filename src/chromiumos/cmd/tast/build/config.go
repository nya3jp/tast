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
	// CheckBuildDeps indicates whether to check that build dependencies are installed and up-to-date in
	// the host sysroot.
	CheckBuildDeps bool
	// InstallPortageDeps controls whether outdated or missing Portage deps are automatically installed.
	// If false, a message is generated with the commands(s) that should be manually run and an error is returned.
	InstallPortageDeps bool
	// CheckDepsCachePath is the path to a JSON file storing cached information to avoid running emerge
	// to check build dependencies when possible. See checkDepsCache for the format.
	CheckDepsCachePath string
}

// Target describes a Go executable package to build and configurations needed to built it.
type Target struct {
	// Pkg is the name of a Go executable package to build.
	Pkg string
	// Arch is the userland architecture to build for. It is usually given by "uname -m", but it can be different
	// if the kernel and the userland use different architectures (e.g. aarch64 kernel with armv7l userland).
	Arch string
	// Workspaces contains paths to Go workspaces (i.e. with "src" subdirectories) containing source code to be compiled.
	// These are placed into the GOPATH environment variable in the listed order.
	Workspaces []string
	// OutDir is the path of the directory to save a built executable to. The executable file name is assigned
	// by "go install" (i.e. it's the last component of the package name).
	OutDir string
}
