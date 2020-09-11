// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"reflect"
	"strconv"
	gotesting "testing"
	"time"

	"github.com/google/subcommands"

	"chromiumos/tast/internal/control"
	"chromiumos/tast/internal/faketlw"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/testing"
)

func TestRunPartialRun(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	// Make the local runner report success, but set a nonexistent path for
	// the remote runner so that it will fail.
	td.cfg.runLocal = true
	td.cfg.runRemote = true
	const testName = "pkg.Test"
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		mw := control.NewMessageWriter(stdout)
		mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 1})
		mw.WriteMessage(&control.EntityStart{Time: time.Unix(2, 0), Info: testing.EntityInfo{Name: testName}})
		mw.WriteMessage(&control.EntityEnd{Time: time.Unix(3, 0), Name: testName})
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

	td.cfg.runLocal = true
	td.cfg.KeyFile = "" // force SSH auth error

	if status, _ := Run(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitFailure {
		t.Errorf("Run() = %v; want %v", status, subcommands.ExitFailure)
	} else if !status.FailedBeforeRun {
		// local()'s initial connection attempt will fail, so we won't try to run tests.
		t.Error("Run() incorrectly reported that failure did not occur before trying to run tests")
	}
}

func TestRunEphemeralDevserver(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	td.cfg.runLocal = true
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		mw := control.NewMessageWriter(stdout)
		mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
		mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		return 0
	}
	td.cfg.devservers = nil // clear the default mock devservers set in newLocalTestData
	td.cfg.useEphemeralDevserver = true

	if status, _ := Run(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitSuccess {
		t.Errorf("Run() = %v; want %v (%v)", status.ExitCode, subcommands.ExitSuccess, td.logbuf.String())
	}

	exp := []string{fmt.Sprintf("http://127.0.0.1:%d", ephemeralDevserverPort)}
	if !reflect.DeepEqual(td.cfg.devservers, exp) {
		t.Errorf("Run() set devserver=%v; want %v", td.cfg.devservers, exp)
	}
}

func TestRunDownloadPrivateBundles(t *gotesting.T) {
	td := newLocalTestData(t)
	defer td.close()

	td.cfg.runLocal = true
	called := false
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		switch args.Mode {
		case runner.RunTestsMode:
			mw := control.NewMessageWriter(stdout)
			mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
			mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		case runner.DownloadPrivateBundlesMode:
			exp := runner.DownloadPrivateBundlesArgs{
				Devservers:        td.cfg.devservers,
				BuildArtifactsURL: td.cfg.buildArtifactsURL,
			}
			if !reflect.DeepEqual(*args.DownloadPrivateBundles, exp) {
				t.Errorf("got args %+v; want %+v", *args.DownloadPrivateBundles, exp)
			}
			called = true
			json.NewEncoder(stdout).Encode(&runner.DownloadPrivateBundlesResult{})
		default:
			t.Errorf("Unexpected args.Mode = %v", args.Mode)
		}
		return 0
	}

	td.cfg.devservers = []string{"http://example.com:8080"}
	td.cfg.downloadPrivateBundles = true

	if status, _ := Run(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitSuccess {
		t.Errorf("Run() = %v; want %v (%v)", status.ExitCode, subcommands.ExitSuccess, td.logbuf.String())
	}
	if !called {
		t.Errorf("Run did not call downloadPrivateBundles")
	}
}

func TestRunTLW(t *gotesting.T) {
	const targetName = "the_dut"

	td := newLocalTestData(t)
	defer td.close()

	host, portStr, err := net.SplitHostPort(td.cfg.Target)
	if err != nil {
		t.Fatal("net.SplitHostPort: ", err)
	}
	port, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		t.Fatal("strconv.ParseUint: ", err)
	}

	// Start a TLW server that resolves "the_dut:22" to the real target addr/port.
	stopFunc, tlwAddr := faketlw.StartWiringServer(t, faketlw.WithDUTPortMap(map[faketlw.NamePort]faketlw.NamePort{
		{Name: targetName, Port: 22}: {Name: host, Port: int32(port)},
	}))
	defer stopFunc()

	td.cfg.runLocal = true
	td.runFunc = func(args *runner.Args, stdout, stderr io.Writer) (status int) {
		mw := control.NewMessageWriter(stdout)
		mw.WriteMessage(&control.RunStart{Time: time.Unix(1, 0), NumTests: 0})
		mw.WriteMessage(&control.RunEnd{Time: time.Unix(2, 0), OutDir: ""})
		return 0
	}
	td.cfg.Target = targetName
	td.cfg.tlwServer = tlwAddr

	if status, _ := Run(context.Background(), &td.cfg); status.ExitCode != subcommands.ExitSuccess {
		t.Errorf("Run() = %v; want %v (%v)", status.ExitCode, subcommands.ExitSuccess, td.logbuf.String())
	}
}

// TODO(crbug.com/982171): Add a test that runs remote tests successfully.
// This may require merging LocalTestData and RemoteTestData into one.
