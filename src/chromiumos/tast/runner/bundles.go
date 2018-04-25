// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"chromiumos/tast/testing"
)

// getBundlesAndTests returns matched tests and paths to the bundles containing them.
func getBundlesAndTests(args *Args) (bundles []string, tests []*testing.Test, err error) {
	if bundles, err = getBundles(args.BundleGlob); err != nil {
		return nil, nil, err
	}
	tests, bundles, err = getTests(bundles, args.bundleArgs)
	return bundles, tests, err
}
