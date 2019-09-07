// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

const tastDir = "/tmp/tast" // base directory where Tast writes files

// trunkDir returns the path to the Chrome OS checkout (within a chroot).
func trunkDir() string {
	// TODO(derat): Should probably check that we're actually in the chroot first.
	return "/mnt/host/source"
}
