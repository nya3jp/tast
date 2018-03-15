// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package run starts local and remote test executables and interprets their output.
package run

import (
	"context"
	"flag"
	"io"
	"os"
	"path/filepath"
	"time"

	"chromiumos/cmd/tast/build"
	"chromiumos/cmd/tast/logging"
	"chromiumos/tast/host"
)

// PrintMode is used to indicate that tests should be printed rather than being run.
type PrintMode int

const (
	// DontPrint indicates that tests should be run instead of being printed.
	DontPrint PrintMode = iota
	// PrintNames indicates that test names should be printed, one per line.
	PrintNames
	// PrintJSON indicates that test details should be printed as JSON.
	PrintJSON
)

const (
	defaultKeyFile = "chromite/ssh_keys/testing_rsa" // default private SSH key within Chrome OS checkout
)

// Config contains shared configuration information for running local and remote tests.
// Additional state is held in unexported fields, and the same Config struct should be reused
// across calls to different functions in this package.
type Config struct {
	// Logger is used to log progress.
	Logger logging.Logger
	// Build controls whether a single test bundle should be rebuilt and (in the case of
	// local tests) pushed to the target device.
	Build bool
	// BuildBundle contains the name of the test bundle to rebuild (e.g. "cros").
	// It is only used if Build is true.
	BuildBundle string
	// BuildCfg is the configuration for building test bundles. It is only used if Build is true.
	BuildCfg build.Config
	// KeyFile is the path to a private SSH key to use to connect to the target device.
	KeyFile string
	// KeyDir is a directory containing private SSH keys (typically $HOME/.ssh).
	KeyDir string
	// Target is the target device for testing, in the form "[<user>@]host[:<port>]".
	Target string
	// Patterns specifies which tests to run.
	Patterns []string
	// ResDir is the path to the directory where test results should be written.
	ResDir string
	// PrintMode controls whether tests should be printed to PrintDest instead of being executed.
	PrintMode PrintMode
	// PrintDest is used as the destination when PrintMode has a value other than DontPrint.
	PrintDest io.Writer

	remoteRunner    string // path to executable that runs remote test bundles
	remoteBundleDir string // dir where packaged remote test bundles are installed
	remoteDataDir   string // dir containing packaged remote test data

	hst *host.SSH // cached SSH connection; may be nil

	msgTimeout time.Duration // timeout for reading control messages; default used if zero
}

// SetFlags adds common run-related flags to f that store values in Config.
// trunkDir is the path to the Chrome OS checkout (within the chroot).
func (c *Config) SetFlags(f *flag.FlagSet, trunkDir string) {
	kf := filepath.Join(trunkDir, defaultKeyFile)
	if _, err := os.Stat(kf); err != nil {
		kf = ""
	}
	f.StringVar(&c.KeyFile, "keyfile", kf, "path to private SSH key")

	kd := filepath.Join(os.Getenv("HOME"), ".ssh")
	if _, err := os.Stat(kd); err != nil {
		kd = ""
	}
	f.StringVar(&c.KeyDir, "keydir", kd, "directory containing SSH keys")

	f.BoolVar(&c.Build, "build", true, "build and push test bundle")
	f.StringVar(&c.BuildBundle, "buildbundle", "cros", "name of test bundle to build")
	f.StringVar(&c.remoteRunner, "remoterunner", "/usr/bin/remote_test_runner", "executable that runs remote test bundles")
	f.StringVar(&c.remoteBundleDir, "remotebundledir", "/usr/libexec/tast/bundles/remote", "directory containing builtin remote test bundles")
	f.StringVar(&c.remoteDataDir, "remotedatadir", "/usr/share/tast/data/remote", "directory containing builtin remote test data")

	// We only need a results dir if we're running tests rather than printing them.
	if c.PrintMode == DontPrint {
		f.StringVar(&c.ResDir, "resultsdir", "", "directory for test results")
	}
}

// Close releases the config's resources (e.g. cached SSH connections).
// It should be called at the completion of testing.
func (c *Config) Close(ctx context.Context) error {
	var err error
	if c.hst != nil {
		err = c.hst.Close(ctx)
		c.hst = nil
	}
	return err
}
