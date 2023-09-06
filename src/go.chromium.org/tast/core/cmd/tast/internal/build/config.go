// Copyright 2017 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package build

import (
	"go.chromium.org/tast/core/errors"
)

// ArchHost represents the host architecture. It can be set in Target.Arch to
// instruct to use the host toolchains.
const ArchHost = "host"

// Config describes a configuration for building an executable package.
type Config struct {
	// CheckBuildDeps indicates whether to check that build dependencies are installed and up-to-date in
	// the host sysroot.
	CheckBuildDeps bool
	// InstallPortageDeps controls whether outdated or missing Portage deps are automatically installed.
	// If false, a message is generated with the commands(s) that should be manually run and an error is returned.
	InstallPortageDeps bool
	// CheckDepsCachePath is the path to a JSON file storing cached information to avoid running emerge
	// to check build dependencies when possible. See checkDepsCache for the format.
	CheckDepsCachePath string
	// TastWorkspace is the path to the Go workspace containing Tast framework. This path is used to perform
	// source compatibility version checks. If it is empty, no check is performed.
	TastWorkspace string
}

// Target describes a Go executable package to build and configurations needed to built it.
type Target struct {
	// Pkg is the name of a Go executable package to build.
	Pkg string
	// Arch is the userland architecture to build for. It is usually given by "uname -m", but it can be different
	// if the kernel and the userland use different architectures (e.g. aarch64 kernel with armv7l userland).
	// If it is ArchHost, the toolchains for the host is used.
	Arch string
	// Workspaces contains paths to Go workspaces (i.e. with "src" subdirectories) containing source code to be compiled.
	// These are placed into the GOPATH environment variable in the listed order.
	Workspaces []string
	// Out is the path to save a built executable to.
	Out string
	// Debug is a flag indicating whether the binary should be built with debug symbols.
	Debug bool
}

// LocalBundlePrefix returns the local bundle prefix for a particular bundle.
func LocalBundlePrefix(bundle string) (string, error) {
	bundles := map[string]string{
		// Default bundle is to cros
		"":              LocalBundlePkgPathPrefix,
		"cros":          LocalBundlePkgPathPrefix,
		"crosint":       "go.chromium.org/tast-tests-private/crosint/local/bundles",
		"crosint_intel": "go.chromium.org/partner-intel-private/crosint_intel/local/bundles",
	}
	prefix, ok := bundles[bundle]
	if !ok {
		return "", errors.Errorf("failed to find prefix for bundle %s", bundle)
	}
	return prefix, nil

}

// RemoteBundlePrefix returns the remnote bundle prefix for a particular bundle.
func RemoteBundlePrefix(bundle string) (string, error) {
	bundles := map[string]string{
		// Default bundle is cros.
		"":              RemoteBundlePkgPathPrefix,
		"cros":          RemoteBundlePkgPathPrefix,
		"crosint":       "go.chromium.org/tast-tests-private/crosint/remote/bundles",
		"crosint_intel": "go.chromium.org/partner-intel-private/crosint_intel/remote/bundles",
	}
	prefix, ok := bundles[bundle]
	if !ok {
		return "", errors.Errorf("failed to find prefix for bundle %s", bundle)
	}
	return prefix, nil
}
