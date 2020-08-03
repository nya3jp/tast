// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
)

// FixtState is the state the framework passes to Prepare and Close.
// TODO(oka): Move the following states to the best place.
// TODO(oka): Determine the best name for the types. e.g. FixtState or FixtureState?
type FixtState struct{}

// FixtAdjustState is the state the framework passes to Adjust.
type FixtAdjustState struct{}

// FixtPostTestState is the state the framework passes to PostTest.
// TODO(oka): Consider if we can use just State.
type FixtPostTestState struct{}

// Fixture provides implementation the fixture registered into the framework.
// TODO(oka): This type might not be referenced by the tests. Consider using Fixture as the name of
// the struct referenced from the tests.
type Fixture interface {
	// Prepare is the method framework calls to set up the fixture.
	//
	// ctx and s allow accessing fixture's metadata.
	// TODO(oka): determine the details.
	//
	// The return value is made available to the direct children of the entity graph, as long as
	// this fixture and a child live in the same process.
	// If any resource is associated with the value (e.g. Chrome browser connection), it must not
	// be released in Adjust and PostTest because descendant fixtures may cache it or construct
	// subresources derived from it (e.g. Chrome tab connection).
	//
	// Prepare is called in descending order (parents to children) when fixtures are nested.
	//
	// Prepare can be called multiple times. This happens when this fixture's Close is called
	// before completing all tests depending on it in the following cases:
	//  - This fixture's Adjust requested it by returning an error.
	//  - Ascendant fixtures' Adjust requested it by returning an error.
	//
	// In any case, Close is called in a pair with a successful Prepare call.
	//
	// Errors in this method are reported as errors of the fixture itself, rather than tests
	// depending on it.
	//
	// If one or more errors are reported in Prepare by s.Error or s.Fatal, all remaining tests
	// depending on this fixture are marked failed without actually running. Close is not called
	// in this case. If s.Fatal is called, Prepare immediately aborts.
	//
	// This method is the best place to do a heavy-weight setup of the system environment, e.g.
	// restarting a Chrome session.
	Prepare(ctx context.Context, s *FixtState) interface{}

	// Adjust is called by the framework before each test.
	//
	// ctx and s allow accessing test's metadata.
	// TODO(oka): determine the details. Note that s doesn't provide Error or Fatal.
	//
	// If errors are reported in Adjust, the following methods are called in order to reset the
	// fixture:
	// TODO(oka): Consider PostTest calls
	//  - Descendant fixtures' Close in ascending order
	//  - This fixture's Close
	//  - This fixture's Prepare
	//  - Descendant fixtures' Prepare in descending order
	// Reporting errors in Adjust is valid when light-weight adjusting doesn't restore the
	// precondition the fixture declares.
	// If Adjust returns error just after Prepare is called, then
	// all the remaining tests depending on this fixture are marked failed without actually running.
	//
	// Adjust is called in descending order (parents to children) when fixtures are nested.
	//
	// In any case, PostTest is called in a pair with a successful Adjust call.
	//
	// This method is the best place to do a light-weight cleanup of the
	// system environment to the original one when the fixture was
	// set up, e.g. closing open Chrome tabs.
	// This method can do a setup for the test runs next. e.g. redirect logs to a file in the
	// test's output directory.
	Adjust(ctx context.Context, s *FixtAdjustState) error

	// PostTest is called by the framework after each test.
	//
	// ctx and s allow accessing test's metadata.
	// TODO(oka): determine the details.
	//
	// PostTest is called in ascending order (children to parent)
	// when fixtures are nested.
	//
	// s.Fatal or s.Error can be used to report errors in PostTest. When s.Fatal is called PostTest
	// immediately aborts. The error is marked as the test failure.
	//
	// This method is always called if Adjust succeeds.
	//
	// If errors are reported in PostTest, it is reported as the test failure.
	//
	// The errors PostTest reports don't affect the order of the fixture methods the framework
	// calls.
	//
	// This method is the best place to tear down changes Adjust made. e.g. close log files in the
	// test output directory.
	PostTest(ctx context.Context, s *FixtPostTestState)

	// Close is called by the framework to tear down the fixture.
	//
	// ctx and s can be used to access fixutre metadata.
	//
	// Close is called in an ascending order (children to parents) when fixtures are nested.
	//
	// Close is always called in a pair with a successful Prepare call.
	//
	// Errors in this method are reported as errors of the fixture itself, rather than tests
	// depending on it.
	//
	// Errors in Close doesn't affect the order of the fixture methods the framework calls.
	// That is, even if this fixture's Close reports errors, its ascendants' Close are still called.
	//
	// This method is the best place to tear down changes Prepare made. e.g. Unenroll enterprise
	// enrollment.
	// Changes that shouldn't hinder healthy execution of succeeding tests are not necessarily to
	// be teared down. e.g. Chrome session can be left open.
	Close(ctx context.Context, s *FixtState)
}
