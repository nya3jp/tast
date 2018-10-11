// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"io"
	"time"

	"chromiumos/tast/faillog"
)

const (
	localTestTimeout = 2 * time.Minute              // default max runtime for each test
	localTestDataDir = "/usr/local/share/tast/data" // default dir for test data
)

// Local implements the main function for local test bundles.
//
// The returned status code should be passed to os.Exit.
func Local(stdin io.Reader, stdout, stderr io.Writer) int {
	args := Args{DataDir: localTestDataDir}
	cfg := runConfig{
		postTestFunc:       faillog.SaveIfError,
		defaultTestTimeout: localTestTimeout,
	}
	return run(context.Background(), stdin, stdout, stderr, &args, &cfg, localBundle)
}
