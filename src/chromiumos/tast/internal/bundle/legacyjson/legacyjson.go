// Copyright 2022 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package legacyjson defines the schema of JSON-based representation of tests
// used only for -dumptests option in test bundles.
package legacyjson

import (
	"time"

	"chromiumos/tast/internal/dep"
)

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
