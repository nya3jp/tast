// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package jsonprotocol defines the schema of JSON-based protocol among
// Tast CLI, test runners and test bundles.
package jsonprotocol

import (
	"time"

	"chromiumos/tast/internal/testing"
)

// BundleType indicates local or remote bundle type.
type BundleType int

const (
	// LocalBundle is local bundle type.
	LocalBundle BundleType = iota
	// RemoteBundle is remote bundle type.
	RemoteBundle
)

// EntityResult contains the results from a single entity.
// Fields are exported so they can be marshaled by the json package.
type EntityResult struct {
	// EntityInfo contains basic information about the entity.
	testing.EntityInfo
	// Errors contains errors encountered while running the entity.
	// If it is empty, the entity passed.
	Errors []EntityError `json:"errors"`
	// Start is the time at which the entity started (as reported by the test bundle).
	Start time.Time `json:"start"`
	// End is the time at which the entity completed (as reported by the test bundle).
	// It may hold the zero value (0001-01-01T00:00:00Z) to indicate that the entity did not complete
	// (typically indicating that the test bundle, test runner, or DUT crashed mid-test).
	// In this case, at least one error will also be present indicating that the entity was incomplete.
	End time.Time `json:"end"`
	// OutDir is the directory into which entity output is stored.
	OutDir string `json:"outDir"`
	// SkipReason contains a human-readable explanation of why the test was skipped.
	// It is empty if the test actually ran.
	SkipReason string `json:"skipReason"`
	// BundleType is the location of the entity; whether it's in a remote or
	// a local bundle. This field should not be used outside run package.
	// TODO(crbug/1189989): Remove this field; drop TestsToRun from
	// Config and add two fields LocalTestsToRun and RemoteTestsToRun to State.
	BundleType BundleType `json:"-"`
}

// EntityError describes an error that occurred while running an entity.
// Most of its fields are defined in the Error struct in chromiumos/tast/testing.
// This struct just adds an additional "Time" field.
type EntityError struct {
	// Time contains the time at which the error occurred (as reported by the test bundle).
	Time time.Time `json:"time"`
	// Error is an embedded struct describing the error.
	testing.Error
}
