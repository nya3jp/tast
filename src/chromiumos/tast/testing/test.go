// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testing provides public API for tests.
package testing

import (
	"chromiumos/tast/internal/testing"
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
