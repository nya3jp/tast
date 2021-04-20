// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dep

import (
	"testing"

	"chromiumos/tast/internal/protocol"
)

func TestCheckDeps(t *testing.T) {
	d := &Deps{Var: []string{"xyz"}}
	f := &protocol.Features{
		CheckDeps: true,
		Vars:      map[string]string{"abc": "def"},
	}
	if _, err := d.Check(f); err == nil {
		t.Error("Check with unsatisfied dependency unexpectedly succeeded")
	}

	// Disable dependency checks.
	f.CheckDeps = false

	if _, err := d.Check(f); err != nil {
		t.Errorf("Check with satisfied dependency failed: %v", err)
	}
}