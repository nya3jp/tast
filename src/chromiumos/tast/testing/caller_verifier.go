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
	// Function names are matched after packages.Normalize .
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
func (v *callerVerifier) verifyAndRegister(f *runtime.Func, pc uintptr) error {
	if !v.funcPattern.MatchString(packages.Normalize(f.Name())) {
		return fmt.Errorf("test registration needs to be done in %s: %s", v.funcPattern, f.Name())
	}

	file, _ := f.FileLine(pc)
	if _, ok := v.files[file]; ok {
		return fmt.Errorf("testing.AddTest can be called at most once in a file: %s", file)
	}
	v.files[file] = struct{}{}

	return nil
}
