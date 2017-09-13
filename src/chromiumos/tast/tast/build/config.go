// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package build

import (
	"flag"
	"path/filepath"

	"chromiumos/tast/tast/logging"
)

const (
	defaultBuildOutDir = "/tmp/tast/build"   // default directory used to store compiled code
	defaultTestDir     = "src/platform/tast" // relative path within Chrome OS checkout to Go workspace with test code
)

// Config describes a configuration for building a test executable.
type Config struct {
	// Logger is used to write informational messages.
	Logger logging.Logger
	// TestWorkspace is the path to the Go workspace where test source code is stored (i.e.
	// containing a top-level directory named "src").
	TestWorkspace string
	// SysGopath is the path to the Go workspace containing source for test executables' emerged
	// dependencies. This is typically /usr/lib/gopath (for the host) or
	// /build/<board>/usr/lib/gopath (for a compiled board). If empty, the directory that appears
	// to have the most-recently-updated source is used.
	SysGopath string
	// Arch is the architecture to build for (as a machine name or processor given by "uname -m").
	Arch string
	// OutDir is the path to a directory where compiled code is stored (after appending arch).
	OutDir string
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
	f.StringVar(&c.SysGopath, "sysgopath", "",
		"Go workspace containing system package source code (guessed if empty)")
	f.StringVar(&c.TestWorkspace, "testdir", filepath.Join(trunkDir, defaultTestDir),
		"Go workspace containing test source code")
}
