// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"io"
	"time"

	"chromiumos/tast/testing"
)

const (
	remoteTestTimeout = 5 * time.Minute // default max runtime for each test
)

// Remote implements the main function for remote test bundles.
//
// clArgs should typically be os.Args[1:].
// The returned status code should be passed to os.Exit.
func Remote(clArgs []string, stdin io.Reader, stdout, stderr io.Writer) int {
	args := Args{}
	cfg := runConfig{
		preTestFunc: func(ctx context.Context, s *testing.State) {
			// Reconnect between tests if needed.
			dt := s.DUT()
			if !dt.Connected(ctx) {
				s.Log("Reconnecting to DUT")
				if err := dt.Connect(ctx); err != nil {
					s.Fatal("Failed to reconnect to DUT: ", err)
				}
			}
		},
		defaultTestTimeout: remoteTestTimeout,
	}

	return run(context.Background(), clArgs, stdin, stdout, stderr, &args, &cfg, remoteBundle)
}
