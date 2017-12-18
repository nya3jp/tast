// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package build

import (
	"flag"
	"path/filepath"
)

const (
	defaultBuildOutDir = "/tmp/tast/build"         // default directory used to store compiled code
	defaultTestDir     = "src/platform/tast-tests" // relative path in checkout to Go workspace containing test bundles
	defaultCommonDir   = "src/platform/tast"       // relative path in checkout to Go workspace containing common code
)

// Config describes a configuration for building a test executable.
type Config struct {
	// TestWorkspace is the path to the Go workspace where test source code is stored
	// (i.e. containing top-level src/chromiumos/tast/{local,remote}/bundles directories).
	TestWorkspace string
	// CommonWorkspace is the path to the Go workspace where common source code is stored
	// (i.e. containing a top-level src/chromiumos/tast/testing directory).
	CommonWorkspace string
	// SysGopath is the path to the Go workspace containing source for test executables'
	// emerged dependencies. This is typically /usr/lib/gopath.
	SysGopath string
	// Arch is the architecture to build for (as a machine name or processor given by "uname -m").
	Arch string
	// OutDir is the path to a directory where compiled code is stored (after appending arch).
	OutDir string
	// PortagePkg is the Portage package that contains the test executable (when tests are
	// included in a system image rather than being compiled by the tast command).
	// If non-empty, BuildTests checks that the package's direct dependencies are installed
	// in the host sysroot before building tests.
	PortagePkg string
}

// OutPath returns the path to a file named fn within cfg's architecture-specific output dir.
func (c *Config) OutPath(fn string) string {
	return filepath.Join(c.OutDir, c.Arch, fn)
}

// SetFlags adds common build-related flags to f that store values in Config.
// trunkDir is the path to the Chrome OS checkout (within the chroot).
func (c *Config) SetFlags(f *flag.FlagSet, trunkDir string) {
	f.StringVar(&c.Arch, "arch", "", "target architecture (per \"uname -m\")")
	f.StringVar(&c.OutDir, "outdir", defaultBuildOutDir, "directory storing build artifacts")
	f.StringVar(&c.SysGopath, "sysgopath", "/usr/lib/gopath",
		"Go workspace containing system package source code")
	f.StringVar(&c.TestWorkspace, "testdir", filepath.Join(trunkDir, defaultTestDir),
		"Go workspace containing test bundles")
	f.StringVar(&c.CommonWorkspace, "commondir", filepath.Join(trunkDir, defaultCommonDir),
		"Go workspace containing common code")
}
