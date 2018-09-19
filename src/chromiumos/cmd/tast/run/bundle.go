// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

// bundleInfo contains information about a test bundle.
// All paths in this struct are relative to the Chrome OS trunk directory.
type bundleInfo struct {
	workspace  string // path to Go workspace containing test bundle source code
	overlayDir string // overlay directory containing bundle ebuild
}

// knownBundles is a map from a test bundle name to its information.
// All known bundles should be listed here.
var knownBundles map[string]bundleInfo

func init() {
	knownBundles = map[string]bundleInfo{
		"cros": {
			workspace:  "src/platform/tast-tests",
			overlayDir: "src/third_party/chromiumos-overlay",
		},
	}
}

func getKnownBundleInfo(name string) *bundleInfo {
	if b, ok := knownBundles[name]; ok {
		return &b
	}
	return nil
}
