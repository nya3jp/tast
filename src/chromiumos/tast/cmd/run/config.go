// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package run starts local and remote test executables and interprets their output.
package run

import (
	"flag"
	"io"
	"path/filepath"
	"time"

	"chromiumos/tast/cmd/build"
	"chromiumos/tast/cmd/logging"
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
	defaultKeyPath = "chromite/ssh_keys/testing_rsa" // default private SSH key within Chrome OS checkout
)

// Config contains shared configuration information for running local and remote tests.
type Config struct {
	// Logger is used to log progress.
	Logger logging.Logger
	// Build controls whether tests should be rebuilt and (in the case of local tests)
	// pushed to the target device.
	Build bool
	// BuildCfg is the configuration for building tests. It is only used if Build is true.
	BuildCfg build.Config
	// KeyFile is the path to a private SSH key to use to connect to the target device.
	KeyFile string
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

	msgTimeout time.Duration // timeout for reading control messages; default used if zero
}

// SetFlags adds common run-related flags to f that store values in Config.
// trunkDir is the path to the Chrome OS checkout (within the chroot).
func (c *Config) SetFlags(f *flag.FlagSet, trunkDir string) {
	f.StringVar(&c.KeyFile, "keyfile", filepath.Join(trunkDir, defaultKeyPath),
		"path to private SSH key")
	f.BoolVar(&c.Build, "build", true, "build and push tests")

	// We only need a results dir if we're running tests rather than printing them.
	if c.PrintMode == DontPrint {
		f.StringVar(&c.ResDir, "resultsdir", "", "directory for test results")
	}
}
