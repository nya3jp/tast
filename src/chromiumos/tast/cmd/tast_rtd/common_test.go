// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"
)

// Common test data for multiple tests.
const (
	reqName1   = "PassedTest1"
	reqName2   = "SkippedTest1"
	reqName3   = "PassedTest2"
	reqName4   = "SkippedTest2"
	reqName5   = "FailedTest1"
	test1      = "launcher.PinAppToShelf.clamshell_mode"
	test2      = "launcher.PinAppToShelf.tablet_mode"
	test3      = "launcher.CreateAndRenameFolder.clamshell_mode"
	test4      = "launcher.CreateAndRenameFolder.tablet_mode"
	test5      = "meta.LocalFail"
	workDir1   = "/tmp/tast/result1"
	workDir2   = "/tmp/tast/result2"
	sinkPort   = 22
	tlsAddress = "192.168.86.81"
	tlsPort    = 2227
	tlwAddress = "192.168.86.109"
	tlwPort    = 2228
	dut1       = "127.0.0.1:2222"
)

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
