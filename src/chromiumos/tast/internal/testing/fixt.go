// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"regexp"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
)

// Fixture represents fixtures to register to the framework.
type Fixture struct {
	// Name is the name of the fixture.
	// Tests and fixtures use the name to specify the fixture.
	// The name must be camelCase starting with a lowercase letter and containing only digits and letters.
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
	// This timeout is by default 0.
	SetUpTimeout time.Duration

	// ResetTimeout is the timeout applied to Reset.
	// Even if fixtures are nested, the timeout is applied only to this stage.
	// This timeout is by default 0.
	ResetTimeout time.Duration

	// PreTestTimeout is the timeout applied to PreTest.
	// Even if fixtures are nested, the timeout is applied only to this stage.
	// This timeout is by default 0.
	PreTestTimeout time.Duration

	// PostTestTimeout is the timeout applied to PostTest.
	// Even if fixtures are nested, the timeout is applied only to this stage.
	// This timeout is by default 0.
	PostTestTimeout time.Duration

	// TearDownTimeout is the timeout applied to TearDown.
	// Even if fixtures are nested, the timeout is applied only to this stage.
	// This timeout is by default 0.
	TearDownTimeout time.Duration

	// ServiceDeps contains a list of RPC service names in local test bundles that this remote fixture
	// will access. This field is valid only for remote fixtures.
	ServiceDeps []string

	// Vars contains the names of runtime variables used to pass out-of-band data to tests.
	// Values are supplied using "tast run -var=name=value", and tests can access values via State.Var.
	Vars []string

	// Data contains paths of data files needed by the fixture, relative to a
	// "data" subdirectory within the directory in which the fixture is registered.
	Data []string

	// Labels contains freeform text labels describing the fixture.
	Labels []string

	// TODO(oka): Add Param fields.
}

func (f *Fixture) instantiate(pkg string) (*FixtureInstance, error) {
	if err := validateFixture(f); err != nil {
		return nil, err
	}
	return &FixtureInstance{
		Pkg:             pkg,
		Name:            f.Name,
		Desc:            f.Desc,
		Contacts:        append([]string(nil), f.Contacts...),
		Impl:            f.Impl,
		Parent:          f.Parent,
		SetUpTimeout:    f.SetUpTimeout,
		ResetTimeout:    f.ResetTimeout,
		PreTestTimeout:  f.PreTestTimeout,
		PostTestTimeout: f.PostTestTimeout,
		TearDownTimeout: f.TearDownTimeout,
		ServiceDeps:     append([]string(nil), f.ServiceDeps...),
		Data:            append([]string(nil), f.Data...),
		Vars:            append([]string(nil), f.Vars...),
		Labels:          append([]string(nil), f.Labels...),
	}, nil
}

// FixtureInstance represents a fixture instance registered to the framework.
//
// FixtureInstance is to Fixture what TestInstance is to Test.
type FixtureInstance struct {
	// Pkg is the package from which the fixture is registered.
	Pkg string

	// Following fields are copied from Fixture.
	Name            string
	Desc            string
	Contacts        []string
	Impl            FixtureImpl
	Parent          string
	SetUpTimeout    time.Duration
	ResetTimeout    time.Duration
	PreTestTimeout  time.Duration
	PostTestTimeout time.Duration
	TearDownTimeout time.Duration
	Data            []string
	ServiceDeps     []string
	Vars            []string
	Labels          []string

	// Bundle is the name of the test bundle this test belongs to.
	// This field is empty initially, and later set when the test is added
	// to testing.Registry.
	Bundle string
}

// Constraints returns EntityConstraints for this fixture.
func (f *FixtureInstance) Constraints() *EntityConstraints {
	return &EntityConstraints{
		allVars: append([]string(nil), f.Vars...),
		allData: append([]string(nil), f.Data...),
	}
}

// EntityProto returns a protocol buffer message representation of f.
func (f *FixtureInstance) EntityProto() *protocol.Entity {
	return &protocol.Entity{
		Type:        protocol.EntityType_FIXTURE,
		Package:     f.Pkg,
		Name:        f.Name,
		Description: f.Desc,
		Fixture:     f.Parent,
		Dependencies: &protocol.EntityDependencies{
			DataFiles: append([]string(nil), f.Data...),
			Services:  append([]string(nil), f.ServiceDeps...),
		},
		Contacts: &protocol.EntityContacts{
			Emails: append([]string(nil), f.Contacts...),
		},
		LegacyData: &protocol.EntityLegacyData{
			Variables: append([]string(nil), f.Vars...),
			Bundle:    f.Bundle,
		},
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
	//
	// Note that SetUpTimeout is by default 0. Change it to have a valid context.
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
	//
	// Note that ResetTimeout is by default 0. Change it to have a valid context.
	Reset(ctx context.Context) error

	// PreTest is called by the framework before each test to do a light-weight set up for the test.
	//
	// The context and state passed to PreTest are associated with the test metadata. For example,
	// testing.ContextOutDir(ctx) and s.OutDir() return the output directory allocated to the test.
	//
	// PreTest is called in descending order (parents to children) when fixtures are nested.
	//
	// s.Error or s.Fatal can be used to report errors in PreTest. When s.Fatal is called PreTest
	// immediately aborts. The error is marked as the test failure.
	//
	// This method is always called if fixture is successfully reset by SetUp or Reset.
	//
	// In any case, PostTest is called in a pair with a successful PreTest call.
	//
	// If errors are reported in PreTest, it is reported as the test failure.
	//
	// If errors are reported in PreTest, the test and PostTest are not run.
	//
	// This method is the best place to do a setup for the test runs next. e.g. redirect logs to a
	// file in the test's output directory.
	//
	// Note that PreTestTimeout is by default 0. Change it to have a valid context.
	PreTest(ctx context.Context, s *FixtTestState)

	// PostTest is called by the framework after each test to tear down changes PreTest made.
	//
	// The context and state passed to PostTest are associated with the test metadata. For example,
	// testing.ContextOutDir(ctx) and s.OutDir() return the output directory allocated to the test.
	//
	// PostTest is called in ascending order (children to parent) when fixtures are nested.
	//
	// s.Error or s.Fatal can be used to report errors in PostTest. When s.Fatal is called PostTest
	// immediately aborts. The error is marked as the test failure.
	//
	// This method is always called if PreTest succeeds.
	//
	// PostTest is always called in a pair with a successful PreTest call.
	//
	// If errors are reported in PostTest, it is reported as the test failure.
	//
	// The errors PostTest reports don't affect the order of the fixture methods the framework
	// calls.
	//
	// This method is the best place to tear down changes PreTest made. e.g. close log files in the
	// test output directory.
	//
	// Note that PostTestTimeout is by default 0. Change it to have a valid context.
	PostTest(ctx context.Context, s *FixtTestState)

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
	//
	// Note that TearDownTimeout is by default 0. Change it to have a valid context.
	TearDown(ctx context.Context, s *FixtState)
}
