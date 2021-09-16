// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package caller provides utilities to inspect the caller of a function.
package caller

import (
	"fmt"
	"path"
	"runtime"
	"strings"

	"chromiumos/tast/internal/packages"
)

// isRedirect returns true if curFN and nextFN are the same function after
// normalization, meaning curFN is a redirect to nextFN.
func isRedirect(curFN, nextFN string) bool {
	return packages.Normalize(curFN) == packages.Normalize(nextFN)
}

// GetWithIgnore is Get with custom ignore function. If ignore returns true,
// the function is ignored on counting the number of skips.
// This method is exported for unit testing.
func GetWithIgnore(skip int, ignore func(curFN, nextFN string) bool) string {
	var stack []string
	for i := 0; i <= skip; i++ {
		pc, _, _, ok := runtime.Caller(len(stack))
		if !ok {
			panic("Could not decide the caller")
		}
		stack = append(stack, runtime.FuncForPC(pc).Name())
		n := len(stack)

		if n >= 2 && ignore(stack[n-1], stack[n-2]) {
			// Ignore the function on counting the number of skips.
			i--
		}
	}
	return stack[len(stack)-1]
}

// Get is the implementation of the same-name public function.
func Get(skip int) string {
	return GetWithIgnore(skip+1, isRedirect)
}

// Check is the implementation of the same-name public function.
func Check(skip int, pkgs []string) {
	caller := Get(skip + 1)

	callerPkg := caller[0:strings.LastIndex(caller, ".")]
	for _, pkg := range pkgs {
		if packages.Normalize(callerPkg) == packages.Normalize(pkg) {
			return
		}
	}
	callee := Get(skip)
	panic(fmt.Sprintf(
		"%s is not allowed to call %s; check the list in %s",
		caller, path.Base(callee), path.Base(callee)))
}
