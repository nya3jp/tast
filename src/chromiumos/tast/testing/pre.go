// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"chromiumos/tast/internal/testing"
)

// Precondition represents a precondition that must be satisfied before a test is run.
// Preconditions must also implement the unexported preconditionImpl interface,
// which contains methods that are only intended to be called by the testing package.
type Precondition = testing.Precondition

// preconditionImpl contains the actual implementation of a Precondition.
// It is unexported since these methods are only intended to be called from within this package.
type preconditionImpl = testing.PreconditionImpl
