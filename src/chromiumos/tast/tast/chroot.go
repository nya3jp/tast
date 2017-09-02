// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
)

// getTrunkDir returns the path to the Chrome OS checkout (within a chroot).
func getTrunkDir() string {
	// TODO(derat): Should probably check that we're actually in the chroot first.
	return filepath.Join(os.Getenv("HOME"), "trunk")
}
