// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fakerunner

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/runner"
)

// CheckArgs compares two runner.RunnerArgs.
func CheckArgs(t *testing.T, args, exp *runner.RunnerArgs) {
	t.Helper()
	if diff := cmp.Diff(args, exp, cmp.AllowUnexported(runner.RunnerArgs{})); diff != "" {
		t.Errorf("RunnerArgs mismatch (-got +want):\n%v", diff)
	}
}
