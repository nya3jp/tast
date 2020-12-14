// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements the tast_rtd executable, used to invoke tast in RTD.
package main

import (
	"bufio"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
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

// runArgs stores arguments to invoke Tast
type runArgs struct {
	target    string   // the url for the target machine.
	patterns  []string // the names of test to be run.
	tlwServer string   // a string consisting tlw address and port.
	resultDir string   // the result directory of the tast run.
}

// newArgs created an argument structure for invoking tast
func newArgs(inv *rtd.Invocation) *runArgs {
	args := runArgs{
		target: inv.Duts[0].TlsDutName, // TODO: Support multiple DUTs for sharding.
	}

	if inv.TestLabServicesConfig != nil && inv.TestLabServicesConfig.TlwAddress != "" {
		args.tlwServer = inv.TestLabServicesConfig.TlwAddress
		if inv.TestLabServicesConfig.TlwPort != 0 {
			args.tlwServer = net.JoinHostPort(args.tlwServer, strconv.Itoa(int(inv.TestLabServicesConfig.TlwPort)))
		}
	}

	for _, r := range inv.Requests {
		args.patterns = append(args.patterns, r.Test)
		if args.resultDir == "" {
			args.resultDir = r.Environment.WorkDir
		}
	}
	return &args
}

// genArgList generates argument list for invoking tast
func genArgList(args *runArgs) []string {
	const runSubcommand = "run"
	const tlwFlag = "-tlwserver"
	const resultDirFlag = "-resultsdir"
	argList := []string{runSubcommand}
	if args.tlwServer != "" {
		argList = append(argList, tlwFlag)
		argList = append(argList, args.tlwServer)
	}
	if args.resultDir != "" {
		argList = append(argList, resultDirFlag)
		argList = append(argList, args.resultDir)
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
		if workDir == args.resultDir {
			continue
		}
		if err := os.RemoveAll(workDir); err != nil {
			return errors.Wrapf(err, "failed to remove working directory %v", workDir)
		}
		if err := os.Symlink(args.resultDir, workDir); err != nil {
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
