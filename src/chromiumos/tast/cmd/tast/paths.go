// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
)

const (
	// base directory where Tast writes files
	tastDir = "/tmp/tast"
	// Path to the src files
	srcDir = "chromiumos"
)

// trunkDir returns the path to the Chrome OS checkout (within a chroot).
func trunkDir() string {
	// TODO(derat): Should probably check that we're actually in the chroot first.
	return filepath.Join(os.Getenv("HOME"), srcDir)
}
