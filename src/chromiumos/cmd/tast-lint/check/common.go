// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"path/filepath"
	"regexp"
)

// testMainPathRegexp matches a file name of a Tast test main file.
var testMainPathRegexp = regexp.MustCompile(`/src/chromiumos/tast/(?:local|remote)/bundles/[^/]+/[^/]+/[^/]+\.go$`)

// isTestMainFile checks if path is a Test test main file.
func isTestMainFile(path string) bool {
	path, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return testMainPathRegexp.MatchString(path)
}
