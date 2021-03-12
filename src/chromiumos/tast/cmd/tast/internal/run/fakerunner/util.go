// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fakerunner

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/runner"
)

// CheckArgs compares two runner.Args.
func CheckArgs(t *testing.T, args, exp *runner.Args) {
	t.Helper()
	if diff := cmp.Diff(args, exp, cmp.AllowUnexported(runner.Args{})); diff != "" {
		t.Errorf("Args mismatch (-got +want):\n%v", diff)
	}
}
