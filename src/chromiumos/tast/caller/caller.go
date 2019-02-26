// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package caller provides utilities to inspect the caller of a function.
package caller

import (
	"fmt"
	"path"
	"runtime"
	"strings"
)

// Get returns the package path-qualified name of a function in the current call stack.
// skip is the number of stack frames to skip, with 0 identifying the frame for Get itself
// and 1 identifying the caller of Get.
func Get(skip int) string {
	pc := make([]uintptr, 1)
	if n := runtime.Callers(skip+1, pc); n < 1 {
		panic("could not decide the caller")
	}

	cf := runtime.CallersFrames(pc)
	f, _ := cf.Next()
	return f.Function
}

// Check examines the current call stack and panics if a function in the specified frame
// does not belong to any package in pkgs.
// skip is the number of stack frames to skip, with 0 identifying the frame for Check itself
// and 1 identifying the caller of Check.
func Check(skip int, pkgs []string) {
	caller := Get(skip + 1)
	callerPkg := strings.SplitN(caller, ".", 2)[0]
	for _, pkg := range pkgs {
		if callerPkg == pkg {
			return
		}
	}
	callee := Get(skip)
	panic(fmt.Sprintf(
		"%s is not allowed to call %s; check the list in %s",
		path.Base(caller), path.Base(callee), path.Base(callee)))
}
