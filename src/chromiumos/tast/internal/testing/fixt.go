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

// FixtureImpl provides implementation of the fixture registered to the framework.
type FixtureImpl interface {
	// Prepare is the method framework calls to set up the fixture.
	//
	// ctx and s allow accessing fixture's metadata.
	// ctx dones't contain information about software dependencies because only tests declare
	// software dependencies.
	//
	// TODO(oka): Consider updating ContextSoftwareDeps API so that it's meaningful for fixture
	// scoped contexts. For tests, ContextSoftwareDeps returns the dependencies the test declares,
	// and utility methods (e.g. arc.New) use it to check that tests declare proper software
	// dependencies. For fixtures, it is uncertain what ContextSoftwareDeps should return. One
	// might think it could return the intersection of the software deps of the tests depending on
	// the fixture, but it doesn't work considering arc.New checks OR condition of the software
	// deps. Still, fixtures can call functions like arc.New that calls ContextSoftwareDeps. It
	// indicates that we need to reconsider ContextSoftwareDeps API.
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
	//
	// If errors are reported in Adjust, the following methods are called in order to reset the
	// fixture:
	//  - Descendant fixtures' Close in ascending order
	//  - This fixture's Close
	//  - This fixture's Prepare
	//  - Descendant fixtures' Prepare in descending order
	// Errors in Adjust doesn't affect ascendant fixtures.
	// After the fixture is reset, Adjust method is called again before running the test.
	// To avoid an infinite loop, if Adjust returns an error just after the fixture is set up, then
	// all the remaining tests depending on this fixture are marked as failed without actually
	// running.
	//
	// Reporting errors in Adjust is valid when light-weight adjusting doesn't restore the
	// precondition the fixture declares.
	//
	// Adjust is called in descending order (parents to children) when fixtures are nested.
	//
	// In any case, PostTest is called in a pair with a successful Adjust call.
	//
	// This method is the best place to do a light-weight cleanup of the
	// system environment to the original one when the fixture was
	// set up, e.g. closing open Chrome tabs.
	// This method also can do a setup for the test runs next. e.g. redirect logs to a file in the
	// test's output directory.
	Adjust(ctx context.Context, s *FixtAdjustState) error

	// PostTest is called by the framework after each test.
	//
	// ctx and s allow accessing test's metadata.
	//
	// PostTest is called in ascending order (children to parent) when fixtures are nested.
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
