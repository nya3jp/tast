// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package caller_test

import (
	"testing"

	"chromiumos/tast/caller"
)

func TestGet(t *testing.T) {
	const exp = "chromiumos/tast/caller_test.TestGet"
	if name := caller.Get(1); name != exp {
		t.Errorf("Get(1) = %q; want %q", name, exp)
	}
}

func TestGetSkip(t *testing.T) {
	const exp = "chromiumos/tast/caller_test.TestGetSkip"
	func() {
		if name := caller.Get(2); name != exp {
			t.Errorf("Get(2) = %q; want %q", name, exp)
		}
	}()
}

func TestCheckPass(t *testing.T) {
	caller.Check(1, []string{"chromiumos/tast/caller_test"})
}

func TestCheckPanic(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Check(1) did not panic")
		}
	}()
	caller.Check(1, []string{"chromiumos/tast/foobar"})
}
