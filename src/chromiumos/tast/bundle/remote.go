// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"os"

	"chromiumos/tast/internal/bundle"
)

// RemoteDelegate injects functions as a part of remote test bundle framework implementation.
//
// Deprecated: Use Delegate.
type RemoteDelegate = Delegate

// RemoteDefault implements the main function for remote test bundles.
//
// Usually the Main function of a remote test bundles should just this function,
// and pass the returned status code to os.Exit.
func RemoteDefault(d Delegate) int {
	stdin, stdout, stderr := lockStdIO()
	return bundle.Remote(os.Args[1:], stdin, stdout, stderr, d)
}
