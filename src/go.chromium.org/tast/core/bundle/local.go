// Copyright 2017 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"os"

	"go.chromium.org/tast/core/internal/bundle"
	"go.chromium.org/tast/core/internal/testing"
)

// LocalDefault implements the main function for local test bundles.
//
// Usually the Main function of a local test bundles should just this function,
// and pass the returned status code to os.Exit.
func LocalDefault(d Delegate) int {
	// Find temp dir if os.TempDir doesn't exist.
	if _, err := os.Stat(os.TempDir()); err != nil {
		for _, tempDir := range []string{"/tmp", "/data/local/tmp"} {
			if _, err := os.Stat(tempDir); err != nil {
				continue
			}
			if err := os.Setenv("TMPDIR", tempDir); err != nil {
				panic("failed to setenv TMPDIR")
			}
			break
		}
	}
	stdin, stdout, stderr := lockStdIO()
	return bundle.Local(os.Args[1:], stdin, stdout, stderr, testing.GlobalRegistry(), d)
}
