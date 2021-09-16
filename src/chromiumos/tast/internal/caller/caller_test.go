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

func TestFuncWithIgnore(t *testing.T) {
	redirect(func() {
		const exp = packages.FrameworkPrefix + "internal/caller_test.TestFuncWithIgnore"
		// GetWithIgnore <- this <- redirect <- TestGetWithIgnore
		if f, _ := caller.FuncWithIgnore(3, func(curFN, nextFN string) bool {
			return false
		}); packages.Normalize(f.Name()) != exp {
			t.Errorf("FuncWithIgnore(3) = %q; want %q", f.Name(), exp)
		}
		if f, _ := caller.FuncWithIgnore(2, func(curFN, nextFN string) bool {
			return packages.Normalize(curFN) == packages.FrameworkPrefix+"internal/caller_test.redirect"
		}); packages.Normalize(f.Name()) != exp {
			t.Errorf("FuncWithIgnore(2) = %q; want %q", f.Name(), exp)
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
