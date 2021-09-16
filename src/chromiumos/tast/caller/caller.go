// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package caller provides utilities to inspect the caller of a function.
package caller

import (
	"chromiumos/tast/internal/caller"
)

// Get returns the package path-qualified name of a function in the current call
// stack. skip is the number of stack frames to skip, with 0 identifying the
// frame for Get itself and 1 identifying the caller of Get.
// Get ignores a function in chromiumos/... calling the same function in
// go.chromium.org/... . For example if chromiumos/tast/foo.Bar calls
// go.chromium.org/tast/foo.Bar, Get ignores chromiumos/tast/foo.Bar.
func Get(skip int) string {
	return caller.Get(skip + 1)
}

// Check examines the current call stack and panics if a function in the
// specified frame does not belong to any package in pkgs.
// Get is used to find the caller with skip.
func Check(skip int, pkgs []string) {
	caller.Check(skip+1, pkgs)
}
