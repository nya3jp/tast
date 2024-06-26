// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/chromiumos/config/go/test/api"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/protocol"
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

	// BugComponent is an id for filing bugs against this fixture, i.e. 'b:1234'. This field is not
	// to be used by the fixtures themselves, but added to metadata used outside of testing.
	BugComponent string

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

	// Params lists the Param structs for parameterized fixtures.
	Params []FixtureParam

	// ServiceDeps contains a list of RPC service names in local test bundles that this remote fixture
	// will access. This field is valid only for remote fixtures.
	ServiceDeps []string

	// Vars contains the names of runtime variables used to pass out-of-band data to tests.
	// Values are supplied using "tast run -var=name=value", and tests can access values via State.Var.
	Vars []string

	// Data contains paths of data files needed by the fixture, relative to a
	// "data" subdirectory within the directory in which the fixture is registered.
	Data []string

	// PrivateAttr contains freeform text private attributres describing the fixture.
	PrivateAttr []string
}

// FixtureParam defines parameters for a parameterized fixture.
type FixtureParam struct {
	// Name is the name of this parameterized fixture.
	// Full name of the fixture will be category.FixtureName.param_name,
	// or category.FixtureName if Name is empty.
	// Name should match with [a-z0-9_]*.
	Name string

	// ExtraContacts is a list of extra email addresses of persons and groups who are familiar
	// with the parameter. At least one personal email address of an active committer
	// should be specified so that we can file bugs or ask for code review.
	ExtraContacts []string

	// BugComponent overrides BugComponent defined in the top-level fixture.
	// This field is for infra/external use only and should not be used
	// or referenced within the fixture code.
	BugComponent string

	// Parent specifies the parent fixture name, or empty if it has no parent.
	// Can only be set if the enclosing fixture doesn't have one already set.
	Parent string

	// SetUpTimeout is the timeout applied to SetUp.
	// Even if fixtures are nested, the timeout is applied only to this stage.
	// Can only be set if the enclosing fixture doesn't have one already set.
	SetUpTimeout time.Duration

	// ResetTimeout is the timeout applied to Reset.
	// Even if fixtures are nested, the timeout is applied only to this stage.
	// Can only be set if the enclosing fixture doesn't have one already set.
	ResetTimeout time.Duration

	// PreTestTimeout is the timeout applied to PreTest.
	// Even if fixtures are nested, the timeout is applied only to this stage.
	// Can only be set if the enclosing fixture doesn't have one already set.
	PreTestTimeout time.Duration

	// PostTestTimeout is the timeout applied to PostTest.
	// Even if fixtures are nested, the timeout is applied only to this stage.
	// Can only be set if the enclosing fixture doesn't have one already set.
	PostTestTimeout time.Duration

	// TearDownTimeout is the timeout applied to TearDown.
	// Even if fixtures are nested, the timeout is applied only to this stage.
	// Can only be set if the enclosing fixture doesn't have one already set.
	TearDownTimeout time.Duration

	// Val is the value which can be retrieved from testing.FixtState.Param() method.
	Val interface{}

	// ExtraServiceDeps contains a list of extra RPC service names in local test bundles
	// that this remote fixture parameter will access.
	// This field is valid only for remote fixtures.
	ExtraServiceDeps []string

	// ExtraData contains paths of extra data files needed by the fixture parameter,
	// relative to a "data" subdirectory within the directory in which the fixture is registered.
	ExtraData []string

	// ExtraPrivateAttr contains extra freeform text private attributes describing the
	// fixture parameter.
	ExtraPrivateAttr []string
}

func (f *Fixture) instantiate(pkg, src string) ([]*FixtureInstance, error) {
	if err := validateFixture(f); err != nil {
		return nil, err
	}
	// Empty Params is equivalent to one Param with all default values.
	ps := f.Params
	if len(ps) == 0 {
		ps = []FixtureParam{{}}
	}

	fis := make([]*FixtureInstance, 0, len(ps))
	for _, p := range ps {
		fi, err := newFixtureInstance(f, pkg, src, &p)
		if err != nil {
			return nil, err
		}
		fis = append(fis, fi)
	}
	return fis, nil
}

func newFixtureInstance(f *Fixture, pkg, src string, p *FixtureParam) (*FixtureInstance, error) {
	name := f.Name
	if p.Name != "" {
		name += "." + p.Name
	}

	contacts := append(f.Contacts, p.ExtraContacts...)

	bugComponent := f.BugComponent
	if p.BugComponent != "" {
		bugComponent = p.BugComponent
	}

	parent := f.Parent
	if p.Parent != "" {
		parent = p.Parent
	}
	if parent == "" && name != TastRootRemoteFixtureName {
		parent = TastRootRemoteFixtureName
	}

	setUpTimeout, err := timeout(p.SetUpTimeout, f.SetUpTimeout, "SetUpTimeout")
	if err != nil {
		return nil, err
	}
	resetTimeout, err := timeout(p.ResetTimeout, f.ResetTimeout, "ResetTimeout")
	if err != nil {
		return nil, err
	}
	preTestTimeout, err := timeout(p.PreTestTimeout, f.PreTestTimeout, "PreTestTimeout")
	if err != nil {
		return nil, err
	}
	postTestTimeout, err := timeout(p.PostTestTimeout, f.PostTestTimeout, "PostTestTimeout")
	if err != nil {
		return nil, err
	}
	tearDownTimeout, err := timeout(p.TearDownTimeout, f.TearDownTimeout, "TearDownTimeout")
	if err != nil {
		return nil, err
	}

	serviceDeps := append(f.ServiceDeps, p.ExtraServiceDeps...)

	data := append(f.Data, p.ExtraData...)

	privateAttr := append(f.PrivateAttr, p.ExtraPrivateAttr...)

	return &FixtureInstance{
		Pkg:             pkg,
		Name:            name,
		Desc:            f.Desc,
		Contacts:        contacts,
		BugComponent:    bugComponent,
		Impl:            f.Impl,
		Parent:          parent,
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		TearDownTimeout: tearDownTimeout,
		Val:             p.Val,
		ServiceDeps:     serviceDeps,
		Data:            data,
		Vars:            f.Vars,
		PrivateAttr:     privateAttr,
		SrcFile:         src,
	}, nil
}

func timeout(paramTimeout, fixtureTimeout time.Duration, timeoutType string) (time.Duration, error) {
	if paramTimeout != 0 {
		if fixtureTimeout != 0 {
			return 0,
				errors.Errorf("Param has %s specified and its enclosing fixture also has %s specified, but only one can be specified",
					timeoutType, timeoutType)
		}
		return paramTimeout, nil
	}
	return fixtureTimeout, nil
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
	BugComponent    string
	Impl            FixtureImpl
	Parent          string
	SetUpTimeout    time.Duration
	ResetTimeout    time.Duration
	PreTestTimeout  time.Duration
	PostTestTimeout time.Duration
	TearDownTimeout time.Duration
	Val             interface{}
	Data            []string
	ServiceDeps     []string
	Vars            []string
	PrivateAttr     []string
	SrcFile         string

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

// WriteFixturesAsProto exports fixture metadata in the protobuf format defined by infra.
func WriteFixturesAsProto(w io.Writer, prefix string, fixtures []*FixtureInstance) error {
	var fms []*api.TastFixtureMetadata
	for _, src := range fixtures {
		fms = append(fms, src.proto(prefix))
	}
	result := &api.TestHarnessMetadataList{
		Values: []*api.TestHarnessMetadata{
			{
				MetadataType: &api.TestHarnessMetadata_TastMetadata_{
					TastMetadata: &api.TestHarnessMetadata_TastMetadata{
						TastFixtureMetadata: fms,
					},
				},
			},
		},
	}
	d, err := proto.Marshal(result)
	if err != nil {
		return errors.Wrap(err, "Failed to marshalize the proto")
	}
	_, err = w.Write(d)
	return err
}

// proto converts test metadata of TestInstance into a protobuf message.
func (f *FixtureInstance) proto(prefix string) *api.TastFixtureMetadata {
	var owners []*api.Contact
	for _, c := range f.Contacts {
		owners = append(owners, &api.Contact{Email: c})
	}
	fm := &api.TastFixtureMetadata{
		Id:           fmt.Sprintf("%s_%s", prefix, f.Name),
		Owners:       owners,
		BugComponent: &api.BugComponent{Value: f.BugComponent},
		PathToFile:   f.SrcFile,
		Parent:       f.Parent,
	}
	return fm
}
