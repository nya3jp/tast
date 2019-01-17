// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Precondition represents a precondition that must be satisfied before a test is run.
type Precondition interface {
	// Register registers the precondition's implementation with the supplied test.
	// The appropriate PreconditionImpl should be passed to t.RegisterPre.
	// This method is invoked automatically and should not be called by test authors.
	Register(t *Test)
}

// PreconditionImpl contains the actual implementation of a Precondition.
//
// The implementation is stored outside the Precondition interface to prevent it from being directly accessed by tests:
//  - Test.Pre holds the required Precondition.
//  - Test.finalize calls Precondition.Register.
//  - Precondition.Register passes PreconditionImpl to Test.RegisterPre.
//  - Test.RegisterPre sets Test.preImpl.
//  - Later, Test.Run uses Test.preImpl to access Prepare, Close, etc.
type PreconditionImpl interface {
	// Prepare is called immediately before starting each test that depends on the precondition.
	// To report an error, the precondition can use either s.Error/Errorf or s.Fatal/Fatalf;
	// either will result in the test not being run. If Prepare reports an error the test will not run,
	// but the precondition object must be left in a state where future calls to Prepare (and Close)
	// can still succeed.
	Prepare(ctx context.Context, s *State)
	// Close is called immediately after completing the final test that depends on the precondition.
	// This method may be called without an earlier call to Prepare in rare cases (e.g. if
	// TestConfig.PreTestFunc fails); preconditions must be able to handle this.
	Close(ctx context.Context, s *State)
	// Timeout returns the amount of time dedicated to Prepare and to Close.
	Timeout() time.Duration
	// String returns a short, underscore-separated name for the precondition.
	// "chrome_logged_in" and "arc_booted" are examples of good names for preconditions
	// defined by the "chrome" and "arc" packages, respectively.
	String() string
}

// CheckPreconditionAccess returns an error if the caller's caller is not permitted to access pre.
// Only test functions (i.e. Test.Func) or init functions can directly access preconditions,
// and tests must additionally list pre in their Test.Pre field during registration.
// This function should be called by pre's accessor function.
func CheckPreconditionAccess(pre Precondition) error {
	// The 0-th frame represents the caller of runtime.Caller, i.e. this function.
	// Skip both it and this function's caller, i.e. the function used to access the precondition.
	pc, fn, ln, ok := runtime.Caller(2)
	if !ok {
		return errors.New("failed to get caller address")
	}
	callerFunc := runtime.FuncForPC(pc)
	if callerFunc == nil {
		return fmt.Errorf("failed to get func for caller at %v", pc)
	}
	callerName := callerFunc.Name()

	for _, tst := range GlobalRegistry().AllTests() {
		ti, err := getTestFuncInfo(tst.Func)
		if err != nil {
			return fmt.Errorf("failed to get func info for %v: %v", tst.Name, err)
		}

		// init() functions need to reference preconditions when registering tests.
		if strings.HasPrefix(callerName, ti.pkg+".init.") {
			return nil
		}

		// Otherwise, tests that stated that they use a precondition are allowed to
		// reference it from their main functions (but not from anywhere else, as doing
		// so makes it hard to reason about a function's dependencies).
		if testFuncName := ti.pkg + "." + ti.name; tst.Pre == pre && testFuncName == callerName {
			return nil
		}
	}

	return fmt.Errorf("accessed from %v at %v:%v "+
		"(must access from testing.Test.Func after declaring in testing.Test.Pre)",
		callerName, filepath.Base(fn), ln)
}
