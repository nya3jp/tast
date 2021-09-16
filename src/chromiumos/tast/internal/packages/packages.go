// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package packages contains utilities to deal with package names in a robust
// manner. This will be useful on ensuring our code being robust against
// package prefix changes.
package packages

import (
	"strings"
)

// OldFrameworkPrefix is the older common framework package prefix.
const OldFrameworkPrefix = "chromiumos/tast/"

// FrameworkPrefix is the newer common framework package prefix.
const FrameworkPrefix = "go.chromium.org/tast/"

// Normalize normalizes old framework package path to a newer corresponding one.
// If the given string doesn't start with OldFrameworkPrefix, Normalize returns
// the unmodified string.
func Normalize(s string) string {
	if !strings.HasPrefix(s, OldFrameworkPrefix) {
		return s
	}
	return FrameworkPrefix + strings.TrimPrefix(s, OldFrameworkPrefix)
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
