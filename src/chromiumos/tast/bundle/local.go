// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"os"
	"time"
)

const (
	localTestTimeout = time.Minute                        // default max runtime for each test
	localTestDataDir = "/usr/local/share/tast/data/local" // default dir for test data
)

// Local implements the main function for local test bundles.
//
// args should typically be os.Args[1:]. The returned status code should be passed to os.Exit.
func Local(args []string) int {
	cfg, status := parseArgs(os.Stdout, args, localTestDataDir, nil)
	if status != statusSuccess || cfg == nil {
		return status
	}
	cfg.defaultTestTimeout = localTestTimeout
	return runTests(context.Background(), cfg)
}
