// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package resultsjson defines the schema of Tast-specific JSON result files
// (results.json).
package resultsjson

import (
	"time"

	"github.com/golang/protobuf/ptypes"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/dep"
	"chromiumos/tast/internal/protocol"
)

// Test represents a test.
type Test struct {
	// See testing.TestInstance for details of the fields.
	Name         string                 `json:"name"`
	Pkg          string                 `json:"pkg"`
	Desc         string                 `json:"desc"`
	Contacts     []string               `json:"contacts"`
	Attr         []string               `json:"attr"`
	SearchFlags  []*protocol.StringPair `json:"searchFlags,omitempty"`
	Data         []string               `json:"data"`
	Vars         []string               `json:"vars,omitempty"`
	VarDeps      []string               `json:"varDeps,omitempty"`
	SoftwareDeps dep.SoftwareDeps       `json:"softwareDeps,omitempty"`
	ServiceDeps  []string               `json:"serviceDeps,omitempty"`
	Fixture      string                 `json:"fixture,omitempty"`
	Timeout      time.Duration          `json:"timeout"`
	Bundle       string                 `json:"bundle,omitempty"`
}

// Error describes an error encountered while running a test.
type Error struct {
	Time   time.Time `json:"time"`
	Reason string    `json:"reason"`
	File   string    `json:"file"`
	Line   int       `json:"line"`
	Stack  string    `json:"stack"`
}

// Result represents the result of a single test.
type Result struct {
	// Test contains basic information about the test.
	Test
	// Errors contains errors encountered while running the entity.
	// If it is empty, the entity passed.
	Errors []Error `json:"errors"`
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

// NewTest creates Test from protocol.Entity.
func NewTest(e *protocol.Entity) (*Test, error) {
	if e.GetType() != protocol.EntityType_TEST {
		return nil, errors.Errorf("BUG: cannot create resultsjson.Test for a non-test entity: %v", e.GetType())
	}

	var timeout time.Duration
	if topb := e.GetLegacyData().GetTimeout(); topb != nil {
		to, err := ptypes.Duration(topb)
		if err != nil {
			return nil, err
		}
		timeout = to
	}
	return &Test{
		Name:         e.GetName(),
		Pkg:          e.GetPackage(),
		Desc:         e.GetDescription(),
		Contacts:     e.GetContacts().GetEmails(),
		Attr:         e.GetAttributes(),
		SearchFlags:  e.GetSearchFlags(),
		Data:         e.GetDependencies().GetDataFiles(),
		Vars:         e.GetLegacyData().GetVariables(),
		VarDeps:      e.GetLegacyData().GetVariableDeps(),
		SoftwareDeps: e.GetLegacyData().GetSoftwareDeps(),
		ServiceDeps:  e.GetDependencies().GetServices(),
		Fixture:      e.GetFixture(),
		Timeout:      timeout,
		Bundle:       e.GetLegacyData().GetBundle(),
	}, nil
}
