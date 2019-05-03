// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"io"
	"path/filepath"
	gotesting "testing"
	"time"

	"github.com/google/subcommands"

	"chromiumos/tast/control"
	"chromiumos/tast/runner"
	"chromiumos/tast/testing"
)

func TestRunPartialRun(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	// Make the local runner report success, but set a nonexistent path for
	// the remote runner so that it will fail.
	const testName = "pkg.Test"
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		mw := control.NewMessageWriter(stdout)
		mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 1})
		mw.WriteMessage(&control.TestStart{Time: time.Unix(2, 0), Test: testing.Test{Name: testName}})
		mw.WriteMessage(&control.TestEnd{Time: time.Unix(3, 0), Name: testName})
		mw.WriteMessage(&control.RunEnd{Time: time.Unix(4, 0), OutDir: ""})
		return 0
	}
	td.cfg.remoteRunner = filepath.Join(td.tempDir, "missing_remote_test_runner")

	status, results := Run(context.Background(), &td.cfg)
	if status.ExitCode != subcommands.ExitFailure {
		t.Errorf("Run() = %v; want %v (%v)", status.ExitCode, subcommands.ExitFailure, td.logbuf.String())
	}
	if status.FailedBeforeRun {
		t.Error("Run() incorrectly reported that failure occurred before trying to run tests")
	}
	if len(results) != 1 {
		t.Errorf("Run() returned results for %d tests; want 1", len(results))
	}
}

func TestRunError(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	td.cfg.KeyFile = "" // force SSH auth error
	if status, _ := Run(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitFailure {
		t.Errorf("Run() = %v; want %v", status, subcommands.ExitFailure)
	} else if !status.FailedBeforeRun {
		// local()'s initial connection attempt will fail, so we won't try to run tests.
		t.Error("Run() incorrectly reported that failure did not occur before trying to run tests")
	}
}
