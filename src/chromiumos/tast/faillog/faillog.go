// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package faillog provides a post-test hook to save "faillog" on test failures.
// A faillog is a collection of log files which can be used to debug test failures.
package faillog

import (
	"os"
	"os/exec"
	"path/filepath"

	"chromiumos/tast/testing"
)

// SaveIfError saves a faillog only if the test has any errors.
func SaveIfError(s *testing.State) {
	if s.HasError() {
		Save(s)
	}
}

// Save saves a faillog unconditionally.
func Save(s *testing.State) {
	dir := filepath.Join(s.OutDir(), "faillog")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}

	saveProcessList(dir)
	saveScreenshot(dir)
}

// saveProcessList saves "ps" output.
func saveProcessList(dir string) {
	path := filepath.Join(dir, "ps.txt")
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	cmd := exec.Command("ps", "auxwwf")
	cmd.Stdout = f
	cmd.Run()
}

// saveScreenshot saves a screenshot.
func saveScreenshot(dir string) {
	path := filepath.Join(dir, "screenshot.png")
	exec.Command("screenshot", path).Run()
}
