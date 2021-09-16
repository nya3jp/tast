// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
	"regexp"
	"runtime"

	"chromiumos/tast/internal/packages"
)

// callerVerifier is to verify testing.AddTest() function callers.
type callerVerifier struct {
	// funcPattern is a regexp pattern to be matched with the caller function.
	funcPattern *regexp.Regexp

	// files is a set of filepaths that are registered.
	files map[string]struct{}
}

// newCallerVerifier returns an instance of the callerVerifier with the given pattern.
func newCallerVerifier(pattern *regexp.Regexp) *callerVerifier {
	return &callerVerifier{
		funcPattern: pattern,
		files:       make(map[string]struct{}),
	}
}

// verifyAndRegister makes sure following things.
// - the function name at the given pc matches with the required pattern.
// - this function is not called twice or more for a file of the pc.
func (v *callerVerifier) verifyAndRegister(pc uintptr) error {
	rf := runtime.FuncForPC(pc)
	if !v.funcPattern.MatchString(rf.Name()) {
		return fmt.Errorf("test registration needs to be done in %s: %s", v.funcPattern, rf.Name())
	}

	file, _ := rf.FileLine(pc)
	if _, ok := v.files[file]; ok {
		return fmt.Errorf("testing.AddTest can be called at most once in a file: %s", file)
	}
	v.files[file] = struct{}{}

	return nil
}

func packageForPC(pc uintptr) string {
	rf := runtime.FuncForPC(pc)
	p, _ := packages.SplitFuncName(rf.Name())
	return p
}
