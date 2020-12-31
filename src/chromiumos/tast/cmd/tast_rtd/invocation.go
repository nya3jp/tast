// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements the tast_rtd executable, used to invoke tast in RTD.
package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/golang/protobuf/proto"
	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"

	"chromiumos/tast/errors"
)

// unmarshalInvocation unmarshals an invocation request and returns a pointer to rtd.Invocation.
func unmarshalInvocation(req []byte) (*rtd.Invocation, error) {
	inv := &rtd.Invocation{}
	if err := proto.Unmarshal(req, inv); err != nil {
		return nil, errors.Wrap(err, "fail to unmarshal invocation data")
	}
	return inv, nil
}

// Command name and flag names.
const (
	runSubcommand              = "run"
	verboseFlag                = "-verbose"
	logTimeFlag                = "-logtime"
	sshRetriesFlag             = "-sshretries"
	downloadDataFlag           = "-downloaddata"
	buildFlag                  = "-build"
	remoteBundlerDirFlag       = "-remotebundledir"
	remoteDataDirFlag          = "-remotedatadir"
	remoteRunnerFlag           = "-remoterunner"
	defaultVarsDirFlag         = "-defaultvarsdir"
	downloadPrivateBundlesFlag = "-downloadprivatebundles"
	devServerFlag              = "-devservers"
	resultsDirFlag             = "-resultsdir"
	tlwServerFlag              = "-tlwserver"
	waitUntilReadyFlag         = "-waituntilready"
	timeOutFlag                = "-timeout"
)

// runArgs stores arguments to invoke Tast
type runArgs struct {
	target    string            // The url for the target machine.
	patterns  []string          // The names of test to be run.
	tastFlags map[string]string // The flags for tast.
	runFlags  map[string]string // The flags for tast run command.
}

// newArgs created an argument structure for invoking tast
func newArgs(inv *rtd.Invocation) *runArgs {

	args := runArgs{
		target: inv.Duts[0].TlsDutName, // TODO: Support multiple DUTs for sharding.
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
		},
	}
	// If it is running inside a RTD, assume bundle directory,
	if exe, err := os.Executable(); err == nil {
		if strings.Contains(exe, "/rtd/") {
			rtdPath := filepath.Dir(exe)
			args.runFlags[remoteBundlerDirFlag] = filepath.Join(rtdPath, "bundles", "romote")
			args.runFlags[remoteDataDirFlag] = filepath.Join(rtdPath, "bundles", "data")
			args.runFlags[remoteRunnerFlag] = filepath.Join(rtdPath, "remote_test_runner")
			args.runFlags[defaultVarsDirFlag] = filepath.Join(rtdPath, "vars")
		}
	}

	if inv.TestLabServicesConfig != nil && inv.TestLabServicesConfig.TlwAddress != "" {
		tlwServer := inv.TestLabServicesConfig.TlwAddress
		if inv.TestLabServicesConfig.TlwPort != 0 {
			tlwServer = net.JoinHostPort(tlwServer, strconv.Itoa(int(inv.TestLabServicesConfig.TlwPort)))
		}
		args.runFlags[tlwServerFlag] = tlwServer
	}

	resultsDir := args.runFlags[resultsDirFlag]
	for _, r := range inv.Requests {
		args.patterns = append(args.patterns, r.Test)
		if resultsDir == "" {
			resultsDir = r.Environment.WorkDir
		}
	}
	args.runFlags[resultsDirFlag] = resultsDir
	return &args
}

// genArgList generates argument list for invoking tast
func genArgList(args *runArgs) (argList []string) {
	for flag, value := range args.tastFlags {
		argList = append(argList, fmt.Sprintf("%v=%v", flag, value))
	}
	argList = append(argList, runSubcommand)
	for flag, value := range args.runFlags {
		argList = append(argList, fmt.Sprintf("%v=%v", flag, value))
	}
	argList = append(argList, args.target)
	argList = append(argList, args.patterns...)
	return argList
}

// invokeTast invoke tast with the parameters based on rtd.Invocation.
func invokeTast(logger *log.Logger, inv *rtd.Invocation) error {
	const path = "/usr/bin/tast"

	if len(inv.Duts) == 0 {
		return errors.New("no DUT is specified")
	}
	if len(inv.Requests) == 0 {
		return errors.New("No test is specified")
	}

	args := newArgs(inv)

	// Create symbolic links to the the first result directory.
	for _, r := range inv.Requests[1:] {
		workDir := r.Environment.WorkDir
		if workDir == "" {
			continue
		}
		resultsDir := args.runFlags[resultsDirFlag]
		if workDir == resultsDir {
			continue
		}
		if err := os.RemoveAll(workDir); err != nil {
			return errors.Wrapf(err, "failed to remove working directory %v", workDir)
		}
		if err := os.Symlink(resultsDir, workDir); err != nil {
			return errors.Wrapf(err, "failed to create symbolic link %v", workDir)
		}
	}
	// Run tast.
	cmd := exec.Command(path, genArgList(args)...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return errors.Wrap(err, "StderrPipe failed")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return errors.Wrap(err, "StdoutPipe failed")
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			logger.Printf("[tast] %v", scanner.Text())
		}
	}()

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			logger.Printf("[tast] %v", scanner.Text())
		}
	}()

	wg.Wait()

	return cmd.Wait()
}
