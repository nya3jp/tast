// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"log"
	"os"
	"testing"

	v2 "go.chromium.org/chromiumos/config/go/api/test/results/v2"
	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"
)

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

	logger := log.New(os.Stderr, "TestMakeResultRequests: ", log.LstdFlags)

	resultRequests, err := makeResultRequests(logger, &inv, []byte(jsonText))
	if err != nil {
		t.Fatal("Failed to unmarshal invocation data: ", err)
	}
	for i, r := range resultRequests {
		if expectedResults[i].Request != r.Request {
			t.Errorf("Got unexpected request name from makeResultRequests got %q; wanted %q", r.Request, expectedResults[i].Request)
		}
		if expectedResults[i].Result.State != r.Result.State {
			t.Errorf("Got unexpected state for test %q from makeResultRequests got %v; wanted %v", r.Request, r.Result.State, expectedResults[i].Result.State)
		}
	}
}
