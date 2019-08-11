// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
	"regexp"
	"runtime"
)

// callerVerifier is to verify testing.AddTest() function callers.
type callerVerifier struct {
	// funcPattern is a regexp pattern to be matched with the caller function.
	funcPattern *regexp.Regexp
}

// newCallerVerifier returns an instance of the callerVerifier with the given pattern.
func newCallerVerifier(pattern *regexp.Regexp) *callerVerifier {
	return &callerVerifier{
		funcPattern: pattern,
	}
}

// verifyAndRegister makes sure following things.
// - the function name at the given pc matches with the required pattern.
func (v *callerVerifier) verifyAndRegister(pc uintptr) error {
	rf := runtime.FuncForPC(pc)
	if !v.funcPattern.MatchString(rf.Name()) {
		return fmt.Errorf("test registration needs to be done in %s: %s", v.funcPattern, rf.Name())
	}

	return nil
}
