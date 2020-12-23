// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"
)

// Common test data for multiple tests.

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
