// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package main

import (
	"fmt"
	"path/filepath"
	"sort"
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
	rtdPath := "/usr/src/rtd"
	expectedArgs := runArgs{
		target:   dut1,
		patterns: []string{test1, test2},
		tastFlags: map[string]string{
			verboseFlag: "true",
			logTimeFlag: "false",
		},
		runFlags: map[string]string{
			sshRetriesFlag:             "2",
			downloadDataFlag:           "batch",
			buildFlag:                  "false",
			downloadPrivateBundlesFlag: "true",
			timeOutFlag:                "3000",
			resultsDirFlag:             workDir1,
			tlwServerFlag:              fmt.Sprintf("%v:%v", tlwAddress, tlwPort),
			remoteBundlerDirFlag:       filepath.Join(rtdPath, "tast", "bundles", "remote"),
			remoteDataDirFlag:          filepath.Join(rtdPath, "tast", "bundles", "data"),
			remoteRunnerFlag:           filepath.Join(rtdPath, "tast", "bin", "remote_test_runner"),
			defaultVarsDirFlag:         filepath.Join(rtdPath, "tast", "vars"),
			keyfile:                    filepath.Join(rtdPath, "tast", "ssh_keys", "testing_rsa"),
		},
	}

	args := newArgs(&inv, rtdPath)
	if diff := cmp.Diff(&expectedArgs, args, cmp.AllowUnexported(runArgs{})); diff != "" {
		t.Errorf("Got unexpected argument from newArgs (-want +got):\n%s", diff)
	}
}

// TestGenArgList makes sure genArgList generates the correct list of argument for tast.
func TestGenArgList(t *testing.T) {
	args := runArgs{
		target:   dut1,
		patterns: []string{test1, test2},
		tastFlags: map[string]string{
			verboseFlag: "true",
			logTimeFlag: "false",
		},
		runFlags: map[string]string{
			sshRetriesFlag:             "2",
			downloadDataFlag:           "batch",
			buildFlag:                  "false",
			downloadPrivateBundlesFlag: "true",
			timeOutFlag:                "3000",
			resultsDirFlag:             workDir1,
			tlwServerFlag:              fmt.Sprintf("%v:%v", tlwAddress, tlwPort),
		},
	}

	var expectedArgList []string
	for key, value := range args.tastFlags {
		expectedArgList = append(expectedArgList, fmt.Sprintf("%v=%v", key, value))
	}
	runIndex := len(expectedArgList)
	expectedArgList = append(expectedArgList, "run")
	for key, value := range args.runFlags {
		expectedArgList = append(expectedArgList, fmt.Sprintf("%v=%v", key, value))
	}
	dutIndex := len(expectedArgList)
	expectedArgList = append(expectedArgList, dut1)
	expectedArgList = append(expectedArgList, test1)
	expectedArgList = append(expectedArgList, test2)

	argList := genArgList(&args)

	// Sort both lists so that we can compare them.
	sort.Sort(sort.StringSlice(expectedArgList[0:runIndex]))
	sort.Sort(sort.StringSlice(argList[0:runIndex]))
	sort.Sort(sort.StringSlice(expectedArgList[runIndex+1 : dutIndex]))
	sort.Sort(sort.StringSlice(argList[runIndex+1 : dutIndex]))

	if diff := cmp.Diff(argList, expectedArgList, cmp.AllowUnexported(runArgs{})); diff != "" {
		t.Errorf("Got unexpected argument from genArgList (-got %v +want %v):\n%s", argList, expectedArgList, diff)
	}
}
