// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements the tast_rtd executable, used to invoke tast in RTD.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	_struct "github.com/golang/protobuf/ptypes/struct"
	v2 "go.chromium.org/chromiumos/config/go/api/test/results/v2"
	rtd "go.chromium.org/chromiumos/config/go/api/test/rtd/v1"
	"google.golang.org/grpc"

	"chromiumos/tast/errors"
)

const (
	resultsFilename = "results.json" // File containing stream of newline-separated JSON EntityResult objects

	skipReasonKey = "skipReason" // Keyword for skip reason in results.json.
	errorsKey     = "errors"     // Keyword for errors in results.json.
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
	if args.resultDir == "" {
		t := time.Now()
		args.resultDir = filepath.Join("/tmp/tast/results", t.Format("20060102-150405"))
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

	// Set up connection with ProgressSink
	psAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(int(inv.ProgressSinkClientConfig.Port)))
	conn, err := grpc.DialContext(context.Background(), psAddr, grpc.WithBlock())
	if err != nil {
		return errors.Wrapf(err, "failed to connect to progress sink %v", psAddr)
	}
	defer conn.Close()

	psClient := rtd.NewProgressSinkClient(conn)

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

	if err := cmd.Wait(); err != nil {
		return errors.Wrap(err, "failed in running tast")
	}

	if err := readTestResults(psClient, inv, args.resultDir); err != nil {
		return errors.Wrap(err, "failed in reading tast test results")
	}

	return nil
}

type testError struct {
	Time   string `json:"time"`
	Reason string `json:"reason"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Stack  string `json:"stack"`
}
type testResult struct {
	// name contains the test name.
	Name string `json:"name"`
	// Errors contains errors encountered while running the entity.
	// If it is empty, the entity passed.
	Errors []testError `json:"errors"`
	// It is empty if the test actually ran.
	SkipReason string `json:"skipReason"`
}

// readTestResults reads resultDir/results.json and reports result through progress API.
func readTestResults(psClient rtd.ProgressSinkClient, inv *rtd.Invocation, resultDir string) error {
	fullPathName := filepath.Join(resultDir, resultsFilename)
	resultFile, err := os.Open(fullPathName)
	if err != nil {
		return errors.Wrapf(err, "failed to open file %v", fullPathName)
	}
	defer resultFile.Close()
	data, err := ioutil.ReadFile(fullPathName)
	if err != nil {
		return errors.Wrapf(err, "fail to read file %v", fullPathName)
	}
	resultRequests, err := makeResultRequests(inv, data)
	if err != nil {
		return errors.Wrapf(err, "fail to report test result from file %v", fullPathName)
	}
	for _, rr := range resultRequests {
		rspn, err := psClient.ReportResult(context.Background(), rr)
		if err != nil {
			return errors.Wrap(err, "failed to report result")
		}
		if rspn.Terminate {
			return nil
		}
	}
	return nil
}

// makeResultRequests parses the json text from testData and prepares test results for progress API to use.
func makeResultRequests(inv *rtd.Invocation, testData []byte) (resultRequests []*rtd.ReportResultRequest, err error) {
	var results []*testResult
	if err := json.Unmarshal(testData, &results); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal data")
	}
	// A map to maintain test names to result structures.
	testResults := map[string]*testResult{}
	for i, r := range results {
		if r.Name == "" {
			return nil, errors.Errorf("Entry %v has empty test name: %+v", i, r)
		}
		testResults[r.Name] = results[i]
	}
	for _, request := range inv.Requests {
		result, ok := testResults[request.Test]
		if !ok {
			// TODO: Discuss what is the best way to handle this case.
			resultRequests = append(resultRequests, missingTestResult(request.Name))
			continue
		}
		if rr := skippedTestResult(request.Name, result); rr != nil {
			resultRequests = append(resultRequests, rr)
			continue
		}
		if rr := failedTestResult(request.Name, result); rr != nil {
			resultRequests = append(resultRequests, rr)
			continue
		}
		resultRequests = append(resultRequests, &rtd.ReportResultRequest{
			Request: request.Name,
			Result:  &v2.Result{State: v2.Result_SUCCEEDED},
		})
	}
	return resultRequests, nil
}

func missingTestResult(requestName string) *rtd.ReportResultRequest {
	// TODO: Discuss what is the best way to handle this case.
	return &rtd.ReportResultRequest{
		Request: requestName,
		Result: &v2.Result{
			State: v2.Result_SKIPPED,
			Errors: []*v2.Result_Error{
				&v2.Result_Error{
					Source:   v2.Result_Error_TEST,
					Severity: v2.Result_Error_WARNING,
					Details: &_struct.Struct{
						Fields: map[string]*_struct.Value{
							skipReasonKey: &_struct.Value{
								Kind: &_struct.Value_StringValue{
									StringValue: "Test was not found",
								},
							},
						},
					},
				},
			},
		},
	}
}

func skippedTestResult(requestName string, result *testResult) *rtd.ReportResultRequest {
	if result.SkipReason != "" {
		return &rtd.ReportResultRequest{
			Request: requestName,
			Result: &v2.Result{
				State: v2.Result_SKIPPED,
				Errors: []*v2.Result_Error{
					&v2.Result_Error{
						Source:   v2.Result_Error_TEST,
						Severity: v2.Result_Error_WARNING,
						Details: &_struct.Struct{
							Fields: map[string]*_struct.Value{
								skipReasonKey: &_struct.Value{
									Kind: &_struct.Value_StringValue{
										StringValue: result.SkipReason,
									},
								},
							},
						},
					},
				},
			},
		}
	}
	return nil
}

func failedTestResult(requestName string, result *testResult) *rtd.ReportResultRequest {
	if len(result.Errors) > 0 {
		rr := rtd.ReportResultRequest{
			Request: requestName,
			Result: &v2.Result{
				State: v2.Result_FAILED,
			},
		}
		for _, e := range result.Errors {
			rr.Result.Errors = append(rr.Result.Errors, &v2.Result_Error{
				Source:   v2.Result_Error_TEST,
				Severity: v2.Result_Error_CRITICAL,
				Details: &_struct.Struct{
					Fields: map[string]*_struct.Value{
						"time": &_struct.Value{
							Kind: &_struct.Value_StringValue{
								StringValue: e.Time,
							},
						},
						"reason": &_struct.Value{
							Kind: &_struct.Value_StringValue{
								StringValue: e.Reason,
							},
						},
						"file": &_struct.Value{
							Kind: &_struct.Value_StringValue{
								StringValue: e.File,
							},
						},
						"line": &_struct.Value{
							Kind: &_struct.Value_NumberValue{
								NumberValue: float64(e.Line),
							},
						},
						"stack": &_struct.Value{
							Kind: &_struct.Value_StringValue{
								StringValue: e.Stack,
							},
						},
					},
				},
			})
		}
		return &rr
	}
	return nil
}
