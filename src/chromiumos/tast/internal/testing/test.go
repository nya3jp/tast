// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"chromiumos/tast/testing/hwdep"
)

const (
	// ExternalLinkSuffix is a file name suffix for external data link files.
	// These are JSON files that can be unmarshaled into the externalLink struct.
	ExternalLinkSuffix = ".external"

	// ExternalErrorSuffix is a file name suffix for external data download error files.
	// An error message is written to the file when we encounter an error downloading
	// the corresponding external data file. This mechanism is used to pass errors from
	// the test runner (which downloads the files) to the test bundle so the bundle
	// can include them in the test's output.
	ExternalErrorSuffix = ".external-error"
)

// TestFunc is the code associated with a test.
type TestFunc func(context.Context, *State)

// Test describes a registration of one or more test instances.
//
// Test can be passed to testing.AddTest to actually register test instances
// to the framework.
//
// In the most basic form where Params field is empty, Test describes exactly
// one test instance. If Params is not empty, multiple test instances are
// generated on registration by merging each testing.Param to the base Test.
type Test struct {
	// Func is the function to be executed to perform the test.
	Func TestFunc

	// Desc is a short one-line description of the test.
	Desc string

	// Contacts is a list of email addresses of persons and groups who are familiar with the test.
	// At least one personal email address of an active committer should be specified so that we can
	// file bugs or ask for code reviews.
	Contacts []string

	// Attr contains freeform text attributes describing the test.
	// See https://chromium.googlesource.com/chromiumos/platform/tast/+/master/docs/test_attributes.md
	// for commonly-used attributes.
	Attr []string

	// Data contains paths of data files needed by the test, relative to a "data" subdirectory within the
	// directory in which Func is located.
	Data []string

	// Vars contains the names of runtime variables used to pass out-of-band data to tests.
	// Values are supplied using "tast run -var=name=value", and tests can access values via State.Var.
	Vars []string

	// SoftwareDeps lists software features that are required to run the test.
	// If any dependencies are not satisfied by the DUT, the test will be skipped.
	// See https://chromium.googlesource.com/chromiumos/platform/tast/+/master/docs/test_dependencies.md
	// for more information about dependencies.
	SoftwareDeps []string

	// HardwareDeps describes hardware features and setup that are required to run the test.
	HardwareDeps hwdep.Deps

	// Pre contains a precondition that must be met before the test is run.
	Pre Precondition

	// Fixture is the name of the fixture the test depends on.
	Fixture string

	// Timeout contains the maximum duration for which Func may run before the test is aborted.
	// This should almost always be omitted when defining tests; a reasonable default will be used.
	// This field is serialized as an integer nanosecond count.
	Timeout time.Duration

	// Params lists the Param structs for parameterized tests.
	Params []Param

	// ServiceDeps contains a list of RPC service names in local test bundles that this remote test
	// will access. This field is valid only for remote tests.
	ServiceDeps []string
}

// Param defines parameters for a parameterized test case.
// See also https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Parameterized-tests
type Param struct {
	// Name is the name of this parameterized test.
	// Full name of the test case will be category.TestFuncName.param_name,
	// or category.TestFuncName if Name is empty.
	// Name should match with [a-z0-9_]*.
	Name string

	// ExtraAttr contains freeform text attributes describing the test,
	// in addition to Attr declared in the enclosing Test.
	ExtraAttr []string

	// ExtraData contains paths of data files needed by the test case of this
	// param in addition to Data declared in the enclosing Test.
	ExtraData []string

	// ExtraSoftwareDeps lists software features that are required to run the test case for this param,
	// in addition to SoftwareDeps in the enclosing Test.
	ExtraSoftwareDeps []string

	// ExtraHardwareDeps describes hardware features and setup that are required to run the test for this
	// param, in addition to HardwareDeps in the enclosing Test.
	ExtraHardwareDeps hwdep.Deps

	// Pre contains a precondition that must be met before the test is run.
	// Can only be set if the enclosing test doesn't have one already set.
	Pre Precondition

	// TODO(oka): Consider adding Fixture.

	// Timeout contains the maximum duration for which Func may run before the test is aborted.
	// Can only be set if the enclosing test doesn't have one already set.
	Timeout time.Duration

	// Val is the value which can be retrieved from testing.State.Param() method.
	Val interface{}
}

// validate performs initial validations of Test.
// Most validations are done while constructing TestInstance from a combination
// of Test and Param in newTestInstance, not in this method, so that we can
// validate fields of the final products. However some validations can be done
// only in this method, e.g. checking consistencies among multiple parameters.
func (t *Test) validate() error {
	if err := validateParams(t.Params); err != nil {
		return err
	}
	return nil
}

func validateParams(params []Param) error {
	if len(params) == 0 {
		return nil
	}

	// Ensure unique param name.
	seen := make(map[string]struct{})
	for _, p := range params {
		name := p.Name
		if _, ok := seen[name]; ok {
			return fmt.Errorf("duplicate param name is found: %s", name)
		}
		seen[name] = struct{}{}
	}

	// Ensure all value assigned to Val should have the same type.
	typ0 := reflect.TypeOf(params[0].Val)
	for _, p := range params {
		typ := reflect.TypeOf(p.Val)
		if typ != typ0 {
			return fmt.Errorf("unmatched Val type: got %v; want %v", typ, typ0)
		}
	}

	return nil
}
