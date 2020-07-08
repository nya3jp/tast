// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"time"

	"chromiumos/tast/testing/hwdep"
)

// AddFixt registers fixt to this bundle
// AddFixt must be called inside init() directly.
func AddFixt(fixt *Fixt) {
	// TODO(oka): implement it.
}

// AddFixtParam registers a fixture instance to the framework by providing parameters to the base
// fixture that
// is registered with AddFixt. It's fine to register the base fixture after calling this function,
// but it's an error to not register the base fixture.
//
// name is the base name of the fixture and params are the parameters added to the corresponding
// Fixt. The following two pseudo codes are semantically equivalent.
//
// - AddFixt({Name:"foo", Params:{X}}); AddFixtParams("foo", {Y})
// - AddFixt({Name:"foo", Params:{X,Y}})
//
// One should use AddFixt whenever possible to register fixture instances.
// AddFixtParams is supposed to be called from a helper function in the same file where the base
// fixture is registered,
// and the helper function is called from init() in other packages. It's an error to call this
// function after initialization.
func AddFixtParam(name string, param Param) {
	// TODO(oka): implement it.
}

// Fixt descirbes a registration of one or more fixture instances.
//
// Fixt can be passed to testing.AddFixt to actually register fixture instances to the framework.
//
// In the most basic form where Params field is empty, Fixt describes exactly
// one fixture instance. If Params is not empty, multiple fixture instances are
// generated on registration by merging each testing.Param to the base Fixt.
//
// Fixtures, tests and services are collectively called entities.
// Fixture provides fixed environment for entities.
//
// Entities can depend on zero or one fixture, and this dependency graph forms trees, whose
// internal nodes are fixtures.
// Before a test or a service is run, framework makes sure its ascendant fixtures are all set up.
//
// Fixt must be defined and registered from the package chromiumos/tast/$location/$category
// where $location is local or remote, and $category is the package name.
// The package should provide implementation of the fixture too.
//
// The data files for the fixture are put under
// chromiumos/tast/$location/bundles/cros/$category/data .
// TODO(oka): consider updating the data file location.
type Fixt struct {
	// Name is the base name of the fixture. Name should match with [a-z_]+ .
	//
	// Name is the identifier of fixtures, and must be gloally unique.
	// Registering multiple Fixt with the same base name causes runtime error.
	Name string
	// Impl is the implementation of the fixture.
	Impl Fixture

	// Timeout is the timeout applied to Prepare, Adjust, and Close
	// individually.
	// Even if fixtures are nested, the timeout is applied only to
	// this stage.
	Timeout time.Duration

	// Desc is the description of the fixture.
	// Desc should contain the following information:
	// - The precondition: the condition this fixture sets up. e.g. Chrome is running.
	// - The postcondition: the condition this fixture may leave off. e.g. Chrome might be still
	// running.
	Desc string

	// Contacts is a list of email addresses of persons and groups who are familiar with the
	// fixture.
	// At least one personal email address of an active committer should be specified so that we can
	// file bugs or ask for code reviews.
	Contacts []string

	// Fixt specifies the parent fixture by its name, or empty if it has no parent.
	//
	// Local fixtures can depend on a remote fixture, but not vice-versa.
	// It's an error to depend on a fixture with the same base name.
	Fixt string

	// Data and the other fields are the fixture's properties.
	// See the description of the Test struct for their meanings.
	Data         []string
	Vars         []string
	SoftwareDeps []string
	HardwareDeps hwdep.Deps
	ServiceDeps  []string

	// Params is used to define multiple fixtures with different parameters. See the description of
	// Param for details.
	//
	// Leaving the field empty is identical to setting []Param{{Name:""}} to the field.
	// For each param p, the name of the corresponding fixture instance is Name "." p.Name, or Name
	// if p.Name is empty.
	// For example, if Name is "chrome" and p.Name is "logged_in", the name of the fixture instance
	// will be "chrome.logged_in".
	Params []Param
}

// Fixture provides implementation of a Fixt.
type Fixture interface {
	// Prepare is called by the framework to set up the fixture.
	//
	// Prepare is called in descending order (parents to children) when fixtures are nested.
	//
	// The return value is made available to the direct children of the entity graph, as long as
	// this fixture and a child live in the same process.
	// If any resource is associated with the value (e.g. Chrome browser connection), it must not
	// be released in Adjust because descendant fixtures may cache it or construct subresources
	// derived from it (e.g. Chrome tab connection).
	//
	// Prepare can be called multiple times. This happens when this fixture's Close is called
	// before completing all tests depending on it in the following cases:
	//  - This fixture's Adjust reported errors
	//  - Ascendant fixtures' Adjust reported errors
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
	//
	// ctx and s allow accessing fixture's metadata.
	//  - s.OutDir() returns fixture's output directory.
	//  - s.DataDir() returns fixture's data directory.
	//  - s.FixtVal() returns parent fixture's value if the parent lives in the same process.
	//  - s.Var, s.ServiceDeps and s.SoftwareDeps return the values the fixture instance declares.
	Prepare(ctx context.Context, s *FixtState) interface{}

	// Adjust is called by the framework before each test.
	//
	// Adjust is called in descending order (parents to children)
	// when fixtures are nested.
	//
	// s.Fatal or s.Error can be used to report errors in Adjust. When s.Fatal is called Adjust
	// immediately aborts.
	// If errors are reported in Adjust, the following methods are called in order to reset the
	// fixture:
	//  - Descendant fixtures' Close in ascending order
	//  - This fixture's Close
	//  - This fixture's Prepare
	//  - Descendant fixtures' Prepare in descending order
	// Reporting errors in Adjust is valid when light-weight adjusting doesn't restore the
	// precondition the fixture declares.
	//
	// Failures in Adjust doesn't make the test itself fail.
	//
	// This method is the best place to do a light-weight cleanup of the
	// system environment to the original one when the fixture was
	// set up, e.g. closing open Chrome tabs.
	//
	// ctx and s allow accessing test's metadata. For example,
	// s.OutDir returns the output directory for the current test.
	Adjust(ctx context.Context, s *State)

	// Close is called by the framework to tear down the fixture.
	//
	// Close is called in a ascending order (children to parents) when fixtures are nested.
	//
	// Errors in this method are reported as errors of the fixture itself, rather than tests
	// depending on it.
	//
	// Even if this fixture's Close reports errors, its ascendants' Close are still called.
	//
	// ctx and s allows accessing fixture's metadata. For example, s.OutDir returns the output
	// directory for the fixture.
	Close(ctx context.Context, s *FixtState)
}
