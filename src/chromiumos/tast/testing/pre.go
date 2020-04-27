// Copyright 2019 The Chromium OS Authors. All rights reserved.
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

// preconditionImpl contains the actual implementation of a Precondition.
// It is unexported since these methods are only intended to be called from within this package.
type preconditionImpl interface {
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

// PreconditionV2 should be registered with testing.RegisterPreV2() in init().
type PreconditionV2 interface {
	// String has to be globally unique.
	// Otherwise tast binally will fail.
	String() string
	// Timeout returns the timeout for this precondition (not including parents).
	Timeout() time.Duration
	// Parent returns the name of the parent precondition or an empty string if no parent.
	// It can be remote precondition's name.
	// TODO(oka): wa may want to specify in where the parent is defined.
	Parent() string
}

// preconditinoV2Impl should be implelented if precondition has a parent.
type preconditionV2Impl interface {
	// Prepare is responsible for achieving the state the precondition aim for.
	// It doesn't have to call parent precondition. It's done by the framework.
	// Prepare isn't called if the parent precondition failed.
	Prepare(ctx context.Context, s *State) interface{}
	// Close is responsible for not leaving garbage Prepare may have created to the system.
	Close(ctx context.Context, s *State)
	// Clean is resopnsible for Close to fulfill its responsibility. It's run before the parent's Prepare().
	// Clean is never called if parent doesn't exist.
	Clean(ctx context.Context, s *State)
}
