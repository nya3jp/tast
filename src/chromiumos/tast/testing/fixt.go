// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"chromiumos/tast/internal/testing"
)

// Fixture represents a fixture that must be satisfied before a test is run.
type Fixture = testing.Fixture

// Fixt represents a fixture that must be registered into the framework using testing.RegisterFixt
// method.
type Fixt = testing.Fixt

type FixtState = testing.FixtState

type FixtTestState = testing.FixtTestState
