// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package packages contains utilities to deal with package names in a robust
// manner. This will be useful on ensuring our code being robust against
// package prefix changes.
package packages

import (
	"go.chromium.org/tast/core/internal/packages"
)

// SplitFuncName splits runtime.Func.Name() into package and function name.
func SplitFuncName(fn string) (fullPkg, name string) {
	return packages.SplitFuncName(fn)
}
