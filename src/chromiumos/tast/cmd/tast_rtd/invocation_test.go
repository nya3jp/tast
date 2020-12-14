// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"
)

const reqName1 = "Name1"
const reqName2 = "Name2"
const test1 = "launcher.PinAppToShelf.clamshell_mode"
const test2 = "launcher.PinAppToShelf.tablet_mode"
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
		patterns:  []string{test1, test2},
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
