// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package packages contains utilities to deal with package names in a robust
// manner. This will be useful on ensuring our code being robust against
// package prefix changes.
package packages

import (
	"fmt"
	"regexp"
	"strings"
)

// FrameworkPrefix is the newer common framework package prefix.
const FrameworkPrefix = "go.chromium.org/tast/core/"

// Normalize normalizes old framework package path to a newer corresponding one.
// Normalize return the unmodified string.
// TODO: b/187792551 -- Remove after issue is closed.
func Normalize(s string) string {
	return s
}

// SplitFuncName splits runtime.Func.Name() into package and function name.
func SplitFuncName(fn string) (fullPkg, name string) {
	lastSlash := strings.LastIndex(fn, "/")
	lastPkgAndFunc := strings.SplitN(fn[lastSlash+1:], ".", 2)
	return fn[0:lastSlash+1] + lastPkgAndFunc[0], lastPkgAndFunc[1]
}

// Same returns true if x and y are identical after normalization.
func Same(x, y string) bool {
	return Normalize(x) == Normalize(y)
}

var srcExpr = regexp.MustCompile(fmt.Sprintf(".*/(?P<path>(src/.*))"))

// SrcPathInTastRepo extract <repo>/<src> from a full path.
// Example:
// ~/chromiumos/src/platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/meta
// will be extracted to
// tast-tests/src/go.chromium.org/tast-tests/cros/local/meta
func SrcPathInTastRepo(fn string) string {
	matches := srcExpr.FindStringSubmatch(fn)
	pathIndex := srcExpr.SubexpIndex("path")
	if pathIndex < 0 || pathIndex > len(matches) {
		// Return as is if there is not match.
		return fn
	}
	return matches[pathIndex]
}
