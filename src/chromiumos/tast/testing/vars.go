// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"go.chromium.org/tast/core/testing"
)

// VarString define a structure for global runtime variables of string type.
type VarString = testing.VarString

// RegisterVarString creates and registers a new VarString
func RegisterVarString(name, defaultValue, desc string) *VarString {
	return testing.RegisterVarString(name, defaultValue, desc)
}
