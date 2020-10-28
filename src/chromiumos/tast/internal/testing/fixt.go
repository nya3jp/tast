// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"regexp"
	"time"

	"chromiumos/tast/errors"
)

// FixtureInfo is a JSON-serializable fixture information other processes need.
type FixtureInfo struct {
	// Name is the name of the fixture.
	Name string `json:"name,omitempty"`
	// Parent is the name of the parent fixture or empty if it has no parent.
	Parent string `json:"parent,omitempty"`
}

// Fixture represents fixtures to register to the framework.
type Fixture struct {
	// Name is the name of the fixture.
	// Tests and fixtures use the name to specify the fixture.
	// TODO(oka): We may want to decide the naming convention of the name, e.g. snake case.
	Name string

	// Desc is the description of the fixture.
	Desc string

	// Contacts is a list of email addresses of persons and groups who are familiar with the
	// fixture. At least one personal email address of an active committer should be specified so
	// that we can file bugs or ask for code review.
	Contacts []string

	// Impl is the implementation of the fixture.
	Impl FixtureImpl

	// Parent specifies the parent fixture name, or empty if it has no parent.
	Parent string

	// SetUpTimeout is the timeout applied to SetUp.
	// Even if fixtures are nested, the timeout is applied only to this stage.
	SetUpTimeout time.Duration

	// ResetTimeout is the timeout applied to Reset.
	// Even if fixtures are nested, the timeout is applied only to this stage.
	ResetTimeout time.Duration

	// TearDownTimeout is the timeout applied to TearDown.
	// Even if fixtures are nested, the timeout is applied only to this stage.
	TearDownTimeout time.Duration

	// ServiceDeps contains a list of RPC service names in local test bundles that this remote fixture
	// will access. This field is valid only for remote fixtures.
	ServiceDeps []string

	// TODO(oka): Add Data, Vars and Param fields.
}

// EntityInfo returns EntityInfo for the fixture.
func (f *Fixture) EntityInfo() *EntityInfo {
	return &EntityInfo{
		Name:        f.Name,
		Desc:        f.Desc,
		Contacts:    append([]string(nil), f.Contacts...),
		ServiceDeps: append([]string(nil), f.ServiceDeps...),
		Fixture:     f.Parent,
		Type:        EntityFixture,
	}
}

// fixtureNameRegexp defines the valid fixture name pattern.
var fixtureNameRegexp = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)

// validateFixture validates a user-supplied Fixture metadata.
func validateFixture(f *Fixture) error {
	if !fixtureNameRegexp.MatchString(f.Name) {
		return errors.Errorf("invalid fixture name: %q", f.Name)
	}
	return nil
}

// FixtureImpl provides implementation of the fixture registered to the framework.
type FixtureImpl interface {
	// SetUp is called by the framework to set up the environment with possibly heavy-weight
	// operations.
	//
	// The context and state passed to SetUp are associated with the fixture metadata. For example,
	// testing.ContextOutDir(ctx) and s.OutDir() return the output directory allocated for the
	// fixture itself. testing.ContextSoftwareDeps(ctx) fails since fixtures can't declare software
	// dependencies.
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
	// The return value is made available to the direct children of this fixture in the entity
	// graph, as long as this fixture and a child live in the same process.
	// If any resource is associated with the value (e.g. Chrome browser connection), it must not
	// be released in Reset, PreTest and PostTest because descendant fixtures may cache it or
	// construct subresources derived from it (e.g. Chrome tab connection).
	//
	// SetUp is called in descending order (parents to children) when fixtures are nested.
	//
	// SetUp can be called multiple times. This happens when this fixture's TearDown is called
	// before completing all tests depending on it in the following cases:
	//  - This fixture's Reset requested it by returning an error.
	//  - Ascendant fixtures' Reset requested it by returning an error.
	// In any case, TearDown is called in a pair with a successful SetUp call.
	//
	// Errors in this method are reported as errors of the fixture itself, rather than tests
	// depending on it.
	//
	// If one or more errors are reported in SetUp by s.Error or s.Fatal, all remaining tests
	// depending on this fixture are marked failed without actually running. TearDown is not called
	// in this case. If s.Fatal is called, SetUp immediately aborts.
	//
	// This method is the best place to do a heavy-weight setup of the system environment, e.g.
	// restarting a Chrome session.
	SetUp(ctx context.Context, s *FixtState) interface{}

	// Reset is called by the framework after each test (except for the last one) to do a
	// light-weight reset of the environment to the original state.
	//
	// The context passed to Reset is associated with the fixture metadata. See SetUp for details.
	//
	// If Reset returns a non-nil error, the framework tears down and re-sets up the fixture to
	// recover. To be accurate, the following methods are called in order:
	//  - Descendant fixtures' TearDown in ascending order
	//  - This fixture's TearDown
	//  - This fixture's SetUp
	//  - Descendant fixtures' SetUp in descending order
	// Consequently, errors Reset returns don't affect ascendant fixtures.
	//
	// Returning an error from Reset is valid when light-weight reset doesn't restore the
	// condition the fixture declares.
	//
	// Reset is called in descending order (parents to children) when fixtures are nested.
	//
	// This method is the best place to do a light-weight cleanup of the system environment to the
	// original one when the fixture was set up, e.g. closing open Chrome tabs.
	Reset(ctx context.Context) error

	// TearDown is called by the framework to tear down the environment SetUp set up.
	//
	// The context and state passed to TearDown are associated with the fixture metadata. See SetUp
	// for details.
	//
	// TearDown is called in an ascending order (children to parents) when fixtures are nested.
	//
	// TearDown is always called in a pair with a successful SetUp call.
	//
	// Errors in this method are reported as errors of the fixture itself, rather than tests
	// depending on it.
	//
	// Errors in TearDown doesn't affect the order of the fixture methods the framework calls.
	// That is, even if this fixture's TearDown reports errors, its ascendants' TearDown are still
	// called.
	//
	// This method is the best place to tear down changes SetUp made. e.g. Unenroll enterprise
	// enrollment.
	// Changes that shouldn't hinder healthy execution of succeeding tests are not necessarily to
	// be teared down. e.g. Chrome session can be left open.
	TearDown(ctx context.Context, s *FixtState)
}
