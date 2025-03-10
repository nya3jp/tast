// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakebundle provides a fake implementation of test bundles.
package fakebundle

import (
	"io"
	"os"
	"path/filepath"
	gotesting "testing"

	"go.chromium.org/tast/core/internal/bundle"
	"go.chromium.org/tast/core/internal/fakeexec"
	"go.chromium.org/tast/core/internal/testing"
)

// Install installs fake test bundles to a temporary directory.
// regs is a map whose key is a bundle name and value is an entity registry.
// On success, it returns a file path glob matching test bundle executables.
// Installed files are cleaned up automatically when the current unit test
// finishes.
func Install(t *gotesting.T, regs ...*testing.Registry) (bundleGlob string) {
	t.Helper()

	dir, err := os.MkdirTemp("", "tast-fakebundles.")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	InstallAt(t, dir, regs...)
	return filepath.Join(dir, "*")
}

// InstallAt is similar to Install, but it installs fake test bundles to the
// specified directory.
func InstallAt(t *gotesting.T, dir string, regs ...*testing.Registry) {
	t.Helper()

	for _, reg := range regs {
		reg := reg
		lo, err := fakeexec.CreateLoopback(filepath.Join(dir, reg.Name()), func(args []string, stdin io.Reader, stdout, stderr io.WriteCloser) int {
			return bundle.Local(args[1:], stdin, stdout, stderr, reg, bundle.Delegate{})
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { lo.Close() })
	}
}
