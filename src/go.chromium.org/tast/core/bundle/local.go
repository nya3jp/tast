// Copyright 2017 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"os"

	"go.chromium.org/tast/core/tastuseonly/bundle"
	"go.chromium.org/tast/core/tastuseonly/testing"
)

// LocalDefault implements the main function for local test bundles.
//
// Usually the Main function of a local test bundles should just this function,
// and pass the returned status code to os.Exit.
func LocalDefault(d Delegate) int {
	stdin, stdout, stderr := lockStdIO()
	return bundle.Local(os.Args[1:], stdin, stdout, stderr, testing.GlobalRegistry(), d)
}
