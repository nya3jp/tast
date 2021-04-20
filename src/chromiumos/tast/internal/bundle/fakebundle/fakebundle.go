// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakebundle provides a fake implementation of test bundles.
package fakebundle

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	gotesting "testing"

	"chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/fakeexec"
	"chromiumos/tast/internal/testing"
)

// Install installs fake test bundles to a temporary directory.
// regs is a map whose key is a bundle name and value is an entity registry.
// On success, it returns a file path glob matching test bundle executables.
// Installed files are cleaned up automatically when the current unit test
// finishes.
func Install(t *gotesting.T, regs map[string]*testing.Registry) (bundleGlob string) {
	t.Helper()

	dir, err := ioutil.TempDir("", "tast-fakebundles.")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	for name, reg := range regs {
		name, reg := name, reg
		lo, err := fakeexec.CreateLoopback(filepath.Join(dir, name), func(args []string, stdin io.Reader, stdout, stderr io.WriteCloser) int {
			return bundle.Local(args[1:], stdin, stdout, stderr, reg, bundle.Delegate{})
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { lo.Close() })
	}
	return filepath.Join(dir, "*")
}
