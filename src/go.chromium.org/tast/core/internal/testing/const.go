// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

// TestDidNotRunMsg is the error message for when a test failed before it started.
// This is a magic string that tast.py treats as a NOT_RUN status.
const TestDidNotRunMsg = "Test did not run"

// TastRootRemoteFixtureName is the name of the root remote fixture which will be run
// when a bundle start.
const TastRootRemoteFixtureName = "tastRootRemoteFixture"
