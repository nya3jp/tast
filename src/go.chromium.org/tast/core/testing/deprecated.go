// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"regexp"

	"go.chromium.org/tast/core/internal/testing"
)

// NewTestGlobRegexp returns a compiled regular expression corresponding to g,
// a glob for matching test names.
//
// DEPRECATED: Tests should not use this function.
func NewTestGlobRegexp(g string) (*regexp.Regexp, error) {
	return testing.NewTestGlobRegexp(g)
}
