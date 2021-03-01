// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

// bundleInfo contains information about a test bundle.
// All paths in this struct are relative to the Chrome OS trunk directory.
type bundleInfo struct {
	workspace string // path to Go workspace containing test bundle source code
}

// knownBundles is a map from a test bundle name to its information.
// All known bundles should be listed here.
var knownBundles map[string]bundleInfo

func init() {
	knownBundles = map[string]bundleInfo{
		"cros": {
			workspace: "src/platform/tast-tests",
		},
		"crosint": {
			workspace: "src/platform/tast-tests-private",
		},
	}
}

// getKnownBundleInfo returns a bundleInfo for a given test bundle.
// TODO(nya): Consider simplifying this function to just return a workspace path.
func getKnownBundleInfo(name string) *bundleInfo {
	if b, ok := knownBundles[name]; ok {
		return &b
	}
	return nil
}
