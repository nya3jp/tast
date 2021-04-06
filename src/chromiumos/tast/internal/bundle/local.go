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
	localTestTimeout = 2 * time.Minute // default max runtime for each test
)

// Local implements the main function for local test bundles.
//
// Main function of local test bundles should call LocalDefault instead.
func Local(clArgs []string, stdin io.Reader, stdout, stderr io.Writer, d Delegate) int {
	args, cfg := newArgsAndStaticConfig(localTestTimeout, d)
	return run(context.Background(), clArgs, stdin, stdout, stderr, args, cfg, localBundle)
}
