// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testing provides public API for tests.
package testing

import (
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/testing"
)

// Test describes a registration of one or more test instances.
//
// Test can be passed to testing.AddTest to actually register test instances
// to the framework.
//
// In the most basic form where Params field is empty, Test describes exactly
// one test instance. If Params is not empty, multiple test instances are
// generated on registration by merging each testing.Param to the base Test.
type Test = testing.Test

// Param defines parameters for a parameterized test case.
// See also https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Parameterized-tests
type Param = testing.Param

// TestInstance represents a test instance registered to the framework.
//
// A test instance is the unit of "tests" exposed to outside of the framework.
// For example, in the command line of the "tast" command, users specify
// which tests to run by names of test instances. Single testing.AddTest call
// may register multiple test instances at once if testing.Test passed to the
// function has non-empty Params field.
type TestInstance = testing.TestInstance

const (
	// LifeCycleProductionReady indicates the test can be run in the lab and is expected to pass.
	// Most tests will be in this stage, and this value will be assumed if no other value is provided.
	LifeCycleProductionReady = testing.LifeCycleProductionReady
	// LifeCycleDisabled indicates that the test should not run in the lab and code will be deleted if not cleaned
	// up in a timely manner.
	LifeCycleDisabled = testing.LifeCycleDisabled
	// LifeCycleInDevelopment indicates that the test is either new or broken. It can still run in the lab
	// (ideally at a reduced frequency) but should not be included in flakiness reports or used to make
	// decisions like release qualification.
	LifeCycleInDevelopment = testing.LifeCycleInDevelopment
	// LifeCycleManualOnly indicates that the test is not meant to be scheduled in the lab - it will only be
	// triggered manually or outside of a lab environment. The code should not be deleted unless test owners
	// do not maintain the test.
	LifeCycleManualOnly = testing.LifeCycleManualOnly
	// LifeCycleOwnerMonitored indicates that the test has inherently ambiguous usage and should not be run by
	// or interpreted by anyone other than the test owner. These tests can run in the lab but should not
	// be run in release-blocking suites or any other situation where a non-owner will need to use the results.
	LifeCycleOwnerMonitored = testing.LifeCycleOwnerMonitored
)

const (
	// SatlabRPCServer is the container:port where Satlab RPC server runs and listens to.
	SatlabRPCServer = "satlab_rpcserver:6003"
)

// StringPair represents a string key-value pair. Typically used for SearchFlags.
type StringPair = protocol.StringPair

// TastRootRemoteFixtureName is the name of the root remote fixture which will be run
// when a bundle start.
const TastRootRemoteFixtureName = testing.TastRootRemoteFixtureName
