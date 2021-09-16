// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package caller provides utilities to inspect the caller of a function.
package caller

import (
	"fmt"
	"path"
	"runtime"

	"chromiumos/tast/internal/packages"
)

// isRedirect returns true if curFN and nextFN are the same function after
// normalization, meaning curFN is a redirect to nextFN.
func isRedirect(curFN, nextFN string) bool {
	return packages.Normalize(curFN) == packages.Normalize(nextFN)
}

// FuncWithIgnore is Get with custom ignore function. If ignore returns true,
// the function is ignored on counting the number of skips.
// This method is exported for unit testing.
func FuncWithIgnore(skip int, ignore func(curFN, nextFN string) bool) (*runtime.Func, uintptr) {
	var callStack []string
	skipCount := 0
	for {
		pc, _, _, ok := runtime.Caller(len(callStack))
		if !ok {
			panic("Could not decide the caller")
		}
		f := runtime.FuncForPC(pc)
		callStack = append(callStack, f.Name())

		if n := len(callStack); n >= 2 && ignore(callStack[n-1], callStack[n-2]) {
			// Ignore the function on counting the number of skips.
			continue
		}
		if skipCount == skip {
			return f, pc
		}
		skipCount++
	}
}

// Get is the implementation of the same-name public function.
func Get(skip int) string {
	f, _ := FuncWithIgnore(skip+1, isRedirect)
	return f.Name()
}

// Func is similar to Get but returns *runtime.Func and program counter of
// the caller.
func Func(skip int) (*runtime.Func, uintptr) {
	return FuncWithIgnore(skip+1, isRedirect)
}

// Check is the implementation of the same-name public function.
// Check uses packages.Same to compare two packages.
func Check(skip int, pkgs []string) {
	caller := Get(skip + 1)

	callerPkg, _ := packages.SplitFuncName(caller)
	for _, pkg := range pkgs {
		if packages.Same(callerPkg, pkg) {
			return
		}
	}
	callee := Get(skip)
	panic(fmt.Sprintf(
		"%s is not allowed to call %s; check the list in %s",
		caller, path.Base(callee), path.Base(callee)))
}
