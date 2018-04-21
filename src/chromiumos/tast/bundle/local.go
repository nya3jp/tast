// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"io"
	"time"
)

const (
	localTestTimeout = time.Minute                        // default max runtime for each test
	localTestDataDir = "/usr/local/share/tast/data/local" // default dir for test data
)

// Local implements the main function for local test bundles.
//
// The returned status code should be passed to os.Exit.
func Local(stdin io.Reader, stdout io.Writer) int {
	cfg, status := readArgs(stdin, stdout, &Args{DataDir: localTestDataDir}, localBundle)
	if status != statusSuccess || cfg == nil {
		return status
	}
	cfg.defaultTestTimeout = localTestTimeout
	return runTests(context.Background(), cfg)
}
