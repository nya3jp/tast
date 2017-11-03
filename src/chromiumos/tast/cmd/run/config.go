// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package run starts local and remote test executables and interprets their output.
package run

import (
	"flag"
	"path/filepath"
	"time"

	"chromiumos/tast/cmd/build"
	"chromiumos/tast/cmd/logging"
)

const (
	defaultKeyPath = "chromite/ssh_keys/testing_rsa" // default private SSH key within Chrome OS checkout
)

// Config contains shared configuration information for running local and remote tests.
type Config struct {
	Logger   logging.Logger // used to log progress
	Build    bool           // rebuild tests and (for local tests) push them to the target device
	BuildCfg build.Config   // configuration for building tests; only used if build was requested
	KeyFile  string         // path to private SSH key to use to connect to target device
	Target   string         // target for testing, in the form "[<user>@]host[:<port>]"
	Patterns []string       // patterns specifying tests to run
	ResDir   string         // directory where test results should be written

	msgTimeout time.Duration // timeout for reading control messages; default used if zero
}

// SetFlags adds common run-related flags to f that store values in Config.
// trunkDir is the path to the Chrome OS checkout (within the chroot).
func (c *Config) SetFlags(f *flag.FlagSet, trunkDir string) {
	f.StringVar(&c.KeyFile, "keyfile", filepath.Join(trunkDir, defaultKeyPath),
		"path to private SSH key")
	f.StringVar(&c.ResDir, "resultsdir", "", "directory for test results")
	f.BoolVar(&c.Build, "build", true, "build and push tests")
}
