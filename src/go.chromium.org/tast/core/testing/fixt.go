// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"go.chromium.org/tast/core/internal/testing"
)

// Fixture describes a fixture registered to the framework.
type Fixture = testing.Fixture

// FixtureParam defines parameters for a parameterized fixture.
type FixtureParam = testing.FixtureParam

// FixtureImpl is an interface fixtures should implement.
type FixtureImpl = testing.FixtureImpl
