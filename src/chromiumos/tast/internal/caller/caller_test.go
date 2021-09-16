// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package caller_test

import (
	"testing"

	"chromiumos/tast/internal/caller"
	"chromiumos/tast/internal/packages"
)

func TestGet(t *testing.T) {
	const exp = "chromiumos/tast/internal/caller_test.TestGet"
	if name := caller.Get(1); name != exp {
		t.Errorf("Get(1) = %q; want %q", name, exp)
	}
}

func TestGetSkip(t *testing.T) {
	const exp = "chromiumos/tast/internal/caller_test.TestGetSkip"
	func() {
		if name := caller.Get(2); name != exp {
			t.Errorf("Get(2) = %q; want %q", name, exp)
		}
	}()
}

func redirect(f func()) {
	f()
}

func TestGetWithIgnore(t *testing.T) {
	redirect(func() {
		const exp = packages.FrameworkPrefix + "internal/caller_test.TestGetWithIgnore"
		// GetWithIgnore <- this <- redirect <- TestGetWithIgnore
		if name := caller.GetWithIgnore(3, func(curFN, nextFN string) bool {
			return false
		}); packages.Normalize(name) != exp {
			t.Errorf("GetWithIsRedirect(3) = %q; want %q", name, exp)
		}
		if name := caller.GetWithIgnore(2, func(curFN, nextFN string) bool {
			return packages.Normalize(curFN) == packages.FrameworkPrefix+"internal/caller_test.redirect"
		}); packages.Normalize(name) != exp {
			t.Errorf("GetWithIsRedirect(2) = %q; want %q", name, exp)
		}
	})
}

func TestCheckPass(t *testing.T) {
	caller.Check(1, []string{packages.OldFrameworkPrefix + "internal/caller_test"})
	caller.Check(1, []string{packages.FrameworkPrefix + "internal/caller_test"})
}

func TestCheckPanic(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Check(1) did not panic")
		}
	}()
	caller.Check(1, []string{"chromiumos/internal/tast/foobar"})
}
