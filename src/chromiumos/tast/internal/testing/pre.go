// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"time"
)

// Precondition represents a precondition that must be satisfied before a test is run.
// Preconditions must also implement the unexported preconditionImpl interface,
// which contains methods that are only intended to be called by the testing package.
type Precondition interface {
	// String returns a short, underscore-separated name for the precondition.
	// "chrome_logged_in" and "arc_booted" are examples of good names for preconditions
	// defined by the "chrome" and "arc" packages, respectively.
	String() string
	// Timeout returns the amount of time dedicated to prepare and close the precondition.
	Timeout() time.Duration

	// We intentionally don't embed preconditionImpl here, as doing so lets tests call Prepare
	// and Close on a Precondition (even though preconditionImpl isn't exported). Instead, we
	// explicitly check that Preconditions implement preconditionImpl in Test.finalize.
}

// PreconditionImpl contains the actual implementation of a Precondition.
type PreconditionImpl interface {
	// Prepare is called immediately before starting each test that depends on the precondition.
	// The returned value will be made available to the test via State.PreValue.
	// To report an error, Prepare can call either s.Error/Errorf or s.Fatal/Fatalf.
	// If an error is reported, the test will not run, but the preconditionImpl must be left
	// in a state where future calls to Prepare (and Close) can still succeed.
	Prepare(ctx context.Context, s *State) interface{}
	// Close is called immediately after completing the final test that depends on the precondition.
	// This method may be called without an earlier call to Prepare in rare cases (e.g. if
	// TestConfig.PreTestFunc fails); preconditions must be able to handle this.
	Close(ctx context.Context, s *State)
}
