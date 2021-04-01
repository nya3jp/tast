// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package jsonprotocol defines the schema of JSON-based protocol among
// Tast CLI, test runners and test bundles.
package jsonprotocol

import (
	"fmt"
	"time"

	"chromiumos/tast/internal/dep"
)

// EntityResult contains the results from a single entity.
// Fields are exported so they can be marshaled by the json package.
type EntityResult struct {
	// EntityInfo contains basic information about the entity.
	EntityInfo
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
}

// EntityError describes an error that occurred while running an entity.
// Most of its fields are defined in the Error struct in chromiumos/tast/testing.
// This struct just adds an additional "Time" field.
type EntityError struct {
	// Time contains the time at which the error occurred (as reported by the test bundle).
	Time time.Time `json:"time"`
	// Error is an embedded struct describing the error.
	Error
}

// Error describes an error encountered while running an entity.
type Error struct {
	Reason string `json:"reason"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Stack  string `json:"stack"`
}

// EntityType represents a type of an entity.
type EntityType int

const (
	// EntityTest represents a test.
	// This must be zero so that an unspecified entity type is a test for
	// protocol compatibility.
	EntityTest EntityType = 0

	// EntityFixture represents a fixture.
	EntityFixture EntityType = 1
)

func (t EntityType) String() string {
	switch t {
	case EntityTest:
		return "test"
	case EntityFixture:
		return "fixture"
	default:
		return fmt.Sprintf("unknown(%d)", int(t))
	}
}

// EntityInfo is a JSON-serializable description of an entity.
type EntityInfo struct {
	// See TestInstance for details of the fields.
	Name         string           `json:"name"`
	Pkg          string           `json:"pkg"`
	Desc         string           `json:"desc"`
	Contacts     []string         `json:"contacts"`
	Attr         []string         `json:"attr"`
	Data         []string         `json:"data"`
	Vars         []string         `json:"vars,omitempty"`
	VarDeps      []string         `json:"varDeps,omitempty"`
	SoftwareDeps dep.SoftwareDeps `json:"softwareDeps,omitempty"`
	ServiceDeps  []string         `json:"serviceDeps,omitempty"`
	Fixture      string           `json:"fixture,omitempty"`
	Timeout      time.Duration    `json:"timeout"`
	Type         EntityType       `json:"entityType,omitempty"`

	// Bundle is the basename of the executable containing the entity.
	// Symlinks are not evaluated.
	Bundle string `json:"bundle,omitempty"`
}

// EntityWithRunnabilityInfo is a JSON-serializable description of information of an entity to be used for listing test.
type EntityWithRunnabilityInfo struct {
	// See TestInstance for details of the fields.
	EntityInfo
	SkipReason string `json:"skipReason"`
}