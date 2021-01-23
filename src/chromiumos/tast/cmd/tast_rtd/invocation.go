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
	"sync"
	"time"

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
	keyfile                    = "-keyfile"
	reportsServer              = "-reports_server"
)

// runArgs stores arguments to invoke Tast
type runArgs struct {
	target    string            // The url for the target machine.
	patterns  []string          // The names of test to be run.
	tastFlags map[string]string // The flags for tast.
	runFlags  map[string]string // The flags for tast run command.
}

// newArgs created an argument structure for invoking tast
func newArgs(inv *rtd.Invocation, rtdPath, reportsServerAddr string) *runArgs {
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
			downloadPrivateBundlesFlag: "false", // Default to "false".
			timeOutFlag:                "3000",
			reportsServer:              reportsServerAddr,
		},
	}
	// If it is running inside a RTD, change default path for tast related directories.
	if rtdPath != "" {
		rtdTastPath := filepath.Join(rtdPath, "tast")
		args.runFlags[remoteBundlerDirFlag] = filepath.Join(rtdTastPath, "bundles", "remote")
		args.runFlags[remoteDataDirFlag] = filepath.Join(rtdTastPath, "bundles", "data")
		args.runFlags[remoteRunnerFlag] = filepath.Join(rtdTastPath, "bin", "remote_test_runner")
		args.runFlags[defaultVarsDirFlag] = filepath.Join(rtdTastPath, "vars")
		args.runFlags[keyfile] = filepath.Join(rtdTastPath, "ssh_keys", "testing_rsa")
	}
	if inv.TestLabServicesConfig != nil && inv.TestLabServicesConfig.TlwAddress != "" {
		tlwServer := inv.TestLabServicesConfig.TlwAddress
		if inv.TestLabServicesConfig.TlwPort != 0 {
			tlwServer = net.JoinHostPort(tlwServer, strconv.Itoa(int(inv.TestLabServicesConfig.TlwPort)))
		}
		args.runFlags[tlwServerFlag] = tlwServer
		// Change downloadPrivateBundlesFlag to "true" if tlwServer is specified.
		args.runFlags[downloadPrivateBundlesFlag] = "true"
	}

	resultsDir := args.runFlags[resultsDirFlag]
	for _, r := range inv.Requests {
		args.patterns = append(args.patterns, r.Test)
		if resultsDir == "" {
			resultsDir = r.Environment.WorkDir
		}
	}

	if resultsDir == "" {
		t := time.Now()
		resultsDir = filepath.Join("/tmp/tast/results", t.Format("20060102-150405"))
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
func invokeTast(logger *log.Logger, inv *rtd.Invocation, rtdPath, reportsServer string) (resultsDir string, err error) {
	// The path to the tast executable in chroot is /usr/bin/tast.
	path := "/usr/bin/tast"
	// The path to the tast executable in RTD is /usr/src/rtd/tast/bin/tast.
	if rtdPath != "" {
		path = filepath.Join(rtdPath, "tast", "bin", "tast")
	}

	if len(inv.Duts) == 0 {
		return "", errors.New("input invocation.duts is empty")
	}
	if len(inv.Requests) == 0 {
		return "", errors.New("input invocation.requests is empty")
	}

	args := newArgs(inv, rtdPath, reportsServer)

	// Make sure the result directory exists.
	resultsDir = args.runFlags[resultsDirFlag]
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return resultsDir, errors.Wrapf(err, "failed to create result directory %v", resultsDir)
	}

	// Create symbolic links to the the first result directory.
	for _, r := range inv.Requests[1:] {
		workDir := r.Environment.WorkDir
		if workDir == "" {
			continue
		}
		if workDir == resultsDir {
			continue
		}
		if err := os.RemoveAll(workDir); err != nil {
			return "", errors.Wrapf(err, "failed to remove working directory %v", workDir)
		}
		// Make sure the parent directory of the symbolic link exists.
		if err := os.MkdirAll(filepath.Dir(workDir), 0755); err != nil {
			return resultsDir, errors.Wrapf(err, "failed to create parent directory of symbolic link %v", workDir)
		}
		if err := os.Symlink(resultsDir, workDir); err != nil {
			return resultsDir, errors.Wrapf(err, "failed to create symbolic link %v", workDir)
		}
	}

	// Run tast.
	cmd := exec.Command(path, genArgList(args)...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", errors.Wrap(err, "StderrPipe failed")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", errors.Wrap(err, "StdoutPipe failed")
	}
	if err := cmd.Start(); err != nil {
		return "", err
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

	return resultsDir, cmd.Wait()
}
