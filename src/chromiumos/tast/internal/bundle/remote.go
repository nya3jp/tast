// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"io"
	"time"
)

const (
	remoteTestTimeout = 5 * time.Minute // default max runtime for each test
)

// Remote implements the main function for remote test bundles.
//
// Main function of remote test bundles should call RemoteDefault instead.
func Remote(clArgs []string, stdin io.Reader, stdout, stderr io.Writer, d Delegate) int {
	args, cfg := newArgsAndStaticConfig(remoteTestTimeout, "", d)
	return run(context.Background(), clArgs, stdin, stdout, stderr, args, cfg, remoteBundle)
}
