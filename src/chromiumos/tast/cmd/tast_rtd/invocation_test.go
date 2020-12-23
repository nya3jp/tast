// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	v2 "go.chromium.org/chromiumos/config/go/api/test/results/v2"
	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"
)

const reqName1 = "PassedTest1"
const reqName2 = "SkippedTest1"
const reqName3 = "PassedTest2"
const reqName4 = "SkippedTest2"
const reqName5 = "FailedTest1"
const test1 = "launcher.PinAppToShelf.clamshell_mode"
const test2 = "launcher.PinAppToShelf.tablet_mode"
const test3 = "launcher.CreateAndRenameFolder.clamshell_mode"
const test4 = "launcher.CreateAndRenameFolder.tablet_mode"
const test5 = "meta.LocalFail"
const workDir1 = "/tmp/tast/result1"
const workDir2 = "/tmp/tast/result2"
const sinkPort = 22
const tlsAddress = "192.168.86.81"
const tlsPort = 2227
const tlwAddress = "192.168.86.109"
const tlwPort = 2228
const dut1 = "127.0.0.1:2222"

var inv = rtd.Invocation{
	Requests: []*rtd.Request{
		{
			Name: reqName1,
			Test: test1,
			Environment: &rtd.Request_Environment{
				WorkDir: workDir1,
			},
		},
		{
			Name: reqName2,
			Test: test2,
			Environment: &rtd.Request_Environment{
				WorkDir: workDir2,
			},
		},
		{
			Name: reqName3,
			Test: test3,
			Environment: &rtd.Request_Environment{
				WorkDir: workDir1,
			},
		},
		{
			Name: reqName4,
			Test: test4,
			Environment: &rtd.Request_Environment{
				WorkDir: workDir2,
			},
		},
		{
			Name: reqName5,
			Test: test5,
			Environment: &rtd.Request_Environment{
				WorkDir: workDir2,
			},
		},
	},
	ProgressSinkClientConfig: &rtd.ProgressSinkClientConfig{
		Port: sinkPort,
	},
	TestLabServicesConfig: &rtd.TLSClientConfig{
		TlsAddress: tlsAddress,
		TlsPort:    tlsPort,
		TlwAddress: tlwAddress,
		TlwPort:    tlwPort,
	},
	Duts: []*rtd.DUT{
		{
			TlsDutName: dut1,
		},
	},
}

// TestUnmarshalInvocation makes sure unmarshalInvocation able to unmarshal invocation data.
func TestUnmarshalInvocation(t *testing.T) {
	buf, err := proto.Marshal(&inv)
	if err != nil {
		t.Fatal("Failed to marshal invocation data:", err)
	}
	result, err := unmarshalInvocation(buf)
	if err != nil {
		t.Fatal("Failed to unmarshal invocation data:", err)
	}
	if !proto.Equal(&inv, result) {
		t.Errorf("Invocation did not match: want %v, got %v", inv, result)
	}
}

// TestNewArgs makes sure newArgs creates the correct arguments for tast.
func TestNewArgs(t *testing.T) {
	expectedArgs := runArgs{
		target:    dut1,
		patterns:  []string{test1, test2, test3, test4, test5},
		tlwServer: fmt.Sprintf("%v:%v", tlwAddress, tlwPort),
		resultDir: workDir1,
	}

	args := newArgs(&inv)
	if diff := cmp.Diff(&expectedArgs, args, cmp.AllowUnexported(runArgs{})); diff != "" {
		t.Errorf("Got unexpected argument from newArgs (-want +got):\n%s", diff)
	}
}

// TestGenArgList makes sure genArgList generates the correct list of argument for tast.
func TestGenArgList(t *testing.T) {
	args := runArgs{
		target:    dut1,
		patterns:  []string{test1, test2},
		tlwServer: fmt.Sprintf("%v:%v", tlwAddress, tlwPort),
		resultDir: workDir1,
	}

	expectedArgList := []string{
		"run",
		"-tlwserver", fmt.Sprintf("%v:%v", tlwAddress, tlwPort),
		"-resultsdir", workDir1,
		dut1,
		test1, test2,
	}
	argList := genArgList(&args)
	if diff := cmp.Diff(expectedArgList, argList, cmp.AllowUnexported(runArgs{})); diff != "" {
		t.Errorf("Got unexpected argument from genArgList (-want +got):\n%s", diff)
	}
}

// TestTestResults makes correct test result requests are generated
func TestMakeResultRequests(t *testing.T) {
	jsonText := `[
		{
		  "name": "launcher.CreateAndRenameFolder.tablet_mode",
		  "pkg": "chromiumos/tast/local/bundles/cros/launcher",
		  "desc": "Renaming Folder In Launcher",
		  "contacts": [
			"user@chromium.org"
		  ],
		  "attr": [
			"group:mainline",
			"informational",
			"name:launcher.CreateAndRenameFolder.tablet_mode",
			"bundle:cros",
			"dep:chrome",
			"dep:tablet_mode"
		  ],
		  "data": null,
		  "softwareDeps": [
			"chrome",
			"tablet_mode"
		  ],
		  "timeout": 120000000000,
		  "errors": null,
		  "start": "2020-12-21T14:02:08.288622615-08:00",
		  "end": "2020-12-21T14:02:08.2886929-08:00",
		  "outDir": "/tmp/tast/results/20201221-140206/tests/launcher.CreateAndRenameFolder.tablet_mode",
		  "skipReason": "missing SoftwareDeps: tablet_mode, DUT does not have an internal display"
		},
		{
		  "name": "launcher.PinAppToShelf.tablet_mode",
		  "pkg": "chromiumos/tast/local/bundles/cros/launcher",
		  "desc": "Using Launcher To Pin Application to Shelf",
		  "contacts": [
			"user@chromium.org"
		  ],
		  "attr": [
			"group:mainline",
			"informational",
			"name:launcher.PinAppToShelf.tablet_mode",
			"bundle:cros",
			"dep:chrome",
			"dep:tablet_mode"
		  ],
		  "data": null,
		  "softwareDeps": [
			"chrome",
			"tablet_mode"
		  ],
		  "timeout": 120000000000,
		  "errors": null,
		  "start": "2020-12-21T14:02:08.288727005-08:00",
		  "end": "2020-12-21T14:02:08.288773181-08:00",
		  "outDir": "/tmp/tast/results/20201221-140206/tests/launcher.PinAppToShelf.tablet_mode",
		  "skipReason": "missing SoftwareDeps: tablet_mode, DUT does not have an internal display"
		},
		{
		  "name": "launcher.CreateAndRenameFolder.clamshell_mode",
		  "pkg": "chromiumos/tast/local/bundles/cros/launcher",
		  "desc": "Renaming Folder In Launcher",
		  "contacts": [
			"user@chromium.org"
		  ],
		  "attr": [
			"group:mainline",
			"informational",
			"name:launcher.CreateAndRenameFolder.clamshell_mode",
			"bundle:cros",
			"dep:chrome"
		  ],
		  "data": null,
		  "softwareDeps": [
			"chrome"
		  ],
		  "timeout": 120000000000,
		  "errors": null,
		  "start": "2020-12-21T14:02:08.289113748-08:00",
		  "end": "2020-12-21T14:02:44.350760265-08:00",
		  "outDir": "/tmp/tast/results/20201221-140206/tests/launcher.CreateAndRenameFolder.clamshell_mode",
		  "skipReason": ""
		},
		{
		  "name": "meta.LocalFail",
		  "pkg": "chromiumos/tast/local/bundles/cros/meta",
		  "desc": "Always fails",
		  "contacts": [
			"tast-owners@google.com"
		  ],
		  "attr": [
			"name:meta.LocalFail",
			"bundle:cros",
			"disabled"
		  ],
		  "data": null,
		  "timeout": 120000000000,
		  "errors": [
			{
			  "time": "2020-12-21T14:02:44.356245945-08:00",
			  "reason": "Failed",
			  "file": "/home/user/trunk/src/platform/tast-tests/src/chromiumos/tast/local/bundles/cros/meta/local_fail.go",
			  "line": 22,
			  "stack": "Failed\n\tat chromiumos/tast/local/bundles/cros/meta.LocalFail (local_fail.go:22)\n\tat chromiumos/tast/internal/planner.runTestWithRoot.func3 (run.go:733)\n\tat chromiumos/tast/internal/planner.safeCall.func2 (safe.go:92)\n\tat runtime.goexit (asm_amd64.s:1357)"
			}
		  ],
		  "start": "2020-12-21T14:02:44.352425644-08:00",
		  "end": "2020-12-21T14:02:44.573035513-08:00",
		  "outDir": "/tmp/tast/results/20201221-140206/tests/meta.LocalFail",
		  "skipReason": ""
		},
		{
		  "name": "launcher.PinAppToShelf.clamshell_mode",
		  "pkg": "chromiumos/tast/local/bundles/cros/launcher",
		  "desc": "Using Launcher To Pin Application to Shelf",
		  "contacts": [
			"user@chromium.org"
		  ],
		  "attr": [
			"group:mainline",
			"informational",
			"name:launcher.PinAppToShelf.clamshell_mode",
			"bundle:cros",
			"dep:chrome"
		  ],
		  "data": null,
		  "softwareDeps": [
			"chrome"
		  ],
		  "timeout": 120000000000,
		  "errors": null,
		  "start": "2020-12-21T14:02:44.573275005-08:00",
		  "end": "2020-12-21T14:03:45.514505311-08:00",
		  "outDir": "/tmp/tast/results/20201221-140206/tests/launcher.PinAppToShelf.clamshell_mode",
		  "skipReason": ""
		}
	  ]`
	expectedResults := []*rtd.ReportResultRequest{
		{
			Request: reqName1,
			Result: &v2.Result{
				State: v2.Result_SUCCEEDED,
			},
		},
		{
			Request: reqName2,
			Result: &v2.Result{
				State: v2.Result_SKIPPED,
			},
		},
		{
			Request: reqName3,
			Result: &v2.Result{
				State: v2.Result_SUCCEEDED,
			},
		},
		{
			Request: reqName4,
			Result: &v2.Result{
				State: v2.Result_SKIPPED,
			},
		},
		{
			Request: reqName5,
			Result: &v2.Result{
				State: v2.Result_FAILED,
			},
		},
	}

	resultRequests, err := makeResultRequests(&inv, []byte(jsonText))
	if err != nil {
		t.Fatal("Failed to unmarshal invocation data:", err)
	}
	for i, r := range resultRequests {
		if expectedResults[i].Request != r.Request {
			t.Errorf("Got unexpected request name from makeResultRequests want %q got %q", expectedResults[i].Request, r.Request)
		}
		if expectedResults[i].Result.State != r.Result.State {
			t.Errorf("Got unexpected state for test %q from makeResultRequests want %v got %v", r.Request, expectedResults[i].Result.State, r.Result.State)
		}
	}
}
