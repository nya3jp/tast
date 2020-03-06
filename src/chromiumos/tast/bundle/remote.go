// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"io"
	"os"
	"time"
)

const (
	remoteTestTimeout = 5 * time.Minute // default max runtime for each test
)

// RemoteDefault implements the main function for remote test bundles.
//
// Usually the Main function of a remote test bundles should just this function,
// and pass the returned status code to os.Exit.
func RemoteDefault() int {
	stdin, stdout, stderr := lockStdIO()
	return Remote(os.Args[1:], stdin, stdout, stderr)
}

// Remote implements the main function for remote test bundles.
//
// Main function of remote test bundles should call RemoteDefault instead.
func Remote(clArgs []string, stdin io.Reader, stdout, stderr io.Writer) int {
	args := Args{}
	cfg := runConfig{
		defaultTestTimeout: remoteTestTimeout,
	}
	return run(context.Background(), clArgs, stdin, stdout, stderr, &args, &cfg, remoteBundle)
}
