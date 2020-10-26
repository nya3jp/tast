// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"chromiumos/tast/bundle"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	ibundle "chromiumos/tast/internal/bundle"
	"chromiumos/tast/internal/runner"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/rpc"
	"chromiumos/tast/ssh"
	"chromiumos/tast/timing"
)

const (
	sshConnectTimeout = 10 * time.Second // timeout for establishing SSH connection to DUT
	sshPingTimeout    = 5 * time.Second  // timeout for checking if SSH connection to DUT is open
	sshRetryInterval  = 5 * time.Second  // minimum time to wait between SSH connection attempts

	defaultLocalRunnerWaitTimeout = 10 * time.Second // default timeout for waiting for local_test_runner to exit
	heartbeatInterval             = time.Second      // interval for heartbeat messages
)

// connectToTarget establishes an SSH connection to the target specified in cfg.
// The connection will be cached in cfg and should not be closed by the caller.
// If a connection is already established, it will be returned.
func connectToTarget(ctx context.Context, cfg *Config) (*ssh.Conn, error) {
	// If we already have a connection, reuse it if it's still open.
	if cfg.hst != nil {
		if err := cfg.hst.Ping(ctx, sshPingTimeout); err == nil {
			return cfg.hst, nil
		}
		cfg.hst = nil
	}

	ctx, st := timing.Start(ctx, "connect")
	defer st.End()
	cfg.Logger.Status("Connecting to target")
	cfg.Logger.Logf("Connecting to %s", cfg.Target)

	o := ssh.Options{
		ConnectTimeout:       sshConnectTimeout,
		ConnectRetries:       cfg.sshRetries,
		ConnectRetryInterval: sshRetryInterval,
		KeyFile:              cfg.KeyFile,
		KeyDir:               cfg.KeyDir,
		WarnFunc:             func(s string) { cfg.Logger.Log(s) },
	}
	if err := ssh.ParseTarget(cfg.Target, &o); err != nil {
		return nil, err
	}

	var err error
	if cfg.hst, err = ssh.New(ctx, &o); err != nil {
		return nil, err
	}

	if cfg.initBootID == "" {
		if cfg.initBootID, err = readBootID(ctx, cfg.hst); err != nil {
			return nil, err
		}
	}

	return cfg.hst, nil
}

// createFixtureClient creates remote fixture client.
func createFixtureClient(ctx context.Context, remoteBundleDir string) (cl ibundle.FixtureServiceClient, clean func(), err error) {
	remoteBundlePath := filepath.Join(remoteBundleDir, "cros")

	if _, err := os.Stat(remoteBundlePath); os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("createFixtureClient: %v", err)
	}
	ctx, ctxCancel := context.WithCancel(ctx)

	cmd := exec.CommandContext(ctx, remoteBundlePath, "-rpc")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		panic("todo")
	}
	// FIX: should we close stdin?

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		panic("todo")
	}
	// FIX: should we close stdout?

	if err := cmd.Start(); err != nil {
		panic("todo")
	}
	clean = func() {
		// Kill the gRPC server.
		ctxCancel()
		if err := cmd.Wait(); err != nil {
			// panic("todo")
		}
	}

	conn, err := rpc.NewPipeClientConn(ctx, stdout, stdin, grpc.WithKeepaliveParams(keepalive.ClientParameters{Time: 10 * time.Second, Timeout: 1 * time.Minute}))
	if err != nil {
		panic("todo")
	}
	// FIX: should we defer to close conn?
	cl = ibundle.NewFixtureServiceClient(conn)
	return cl, clean, nil
}

type localTestsCategorizer func([]*testing.EntityInfo) map[string]map[string][]*testing.EntityInfo

// createLocalTestCategorizer creates a function which categorizes given local tests by the bundle
// name and the remote fixture name tests depend on.
func createLocalTestCategorizer(ctx context.Context, cfg *Config, hst *ssh.Conn) (localTestsCategorizer, error) {
	// TODO: move this to run/list_tests.go ?
	var remoteFixts runner.ListFixturesResult
	if err := runTestRunnerCommand(remoteRunnerCommand(ctx, cfg), &runner.Args{
		Mode: runner.ListFixturesMode,
		ListFixtures: &runner.ListFixturesArgs{
			BundleGlob: cfg.remoteBundleGlob(),
		},
	},
		&remoteFixts,
	); err != nil {
		panic(err) // here,
	}
	log.Printf("Got remoteFixts = %#v", remoteFixts) // OK.

	var localFixts runner.ListFixturesResult
	if err := runTestRunnerCommand(localRunnerCommand(ctx, cfg, hst), &runner.Args{
		Mode: runner.ListFixturesMode,
		ListFixtures: &runner.ListFixturesArgs{
			BundleGlob: cfg.localBundleGlob(),
		},
	},
		&localFixts,
	); err != nil {
		panic(err)
	}
	log.Printf("Got localFixts = %#v", localFixts) // OK.

	if len(remoteFixts.Fixtures) > 1 { // bug
		return nil, fmt.Errorf("BUG: there are > 1 remote bundles")
	}
	remoteFixtSet := make(map[string]bool)
	for _, fs := range remoteFixts.Fixtures {
		for _, f := range fs {
			remoteFixtSet[f.Name] = true
			if f.Parent != "" {
				panic("nested remote fixtures are not supported")
			}
		}
	}
	localFixtParent := make(map[string]map[string]string) // bundle -> fixt -> parent
	for b, fs := range localFixts.Fixtures {
		localFixtParent[b] = make(map[string]string)
		for _, f := range fs {
			localFixtParent[b][f.Name] = f.Parent
		}
	}

	log.Printf("Got remote fixture set %v", remoteFixtSet)

	var findRemoteF func(string, string) string
	findRemoteF = func(fixt, bundle string) string {
		if fixt == "" {
			return ""
		}
		// Found remote fixture.
		if remoteFixtSet[fixt] {
			if _, ok := localFixtParent[bundle][fixt]; ok {
				panic("same name fixt in local and remote")
			}
			return fixt
		}
		p, ok := localFixtParent[bundle][fixt]
		if !ok {
			log.Panicf("No such remote fixture %q", fixt)
		}
		return findRemoteF(p, bundle)
	}

	// Categorize local tests by bundle and remote fixture dependency.
	categorizeLocalTests := func(localTests []*testing.EntityInfo) map[string]map[string][]*testing.EntityInfo {
		res := make(map[string]map[string][]*testing.EntityInfo) // bundle -> remote fixt -> tests
		for _, t := range localTests {
			if res[t.Bundle] == nil {
				res[t.Bundle] = make(map[string][]*testing.EntityInfo)
			}
			rf := findRemoteF(t.Fixture, t.Bundle)
			res[t.Bundle][rf] = append(res[t.Bundle][rf], t)
		}
		return res
	}
	return categorizeLocalTests, nil
}

// runFixtureAndTests runs fixture methods before and after runTests.
func runFixtureAndTests(ctx context.Context, cfg *Config, cl ibundle.FixtureServiceClient, r string, runTests func(fixtErr []string) error) error {
	// TODO(oka): Currently we run fixture no matter if the tests dependeing on it
	// are all skipped. We don't know whether tests are skipped until we actually run them.
	// Also, if fixture SetUp fails, all the tests depending on them are marked as failure,
	// even if the tests will be skipped.
	// To fix the issues consider collecting whether the tests are skipped beforehand.

	// TODO: Consider having a central place to manage log files directory structure.
	fixtResDir := filepath.Join(cfg.ResDir, "fixtures", r)
	// TODO: rename testLogFilename to entityLogFilename ?
	fixtLogPath := filepath.Join(fixtResDir, testLogFilename)

	if err := os.MkdirAll(filepath.Dir(fixtLogPath), 0755); err != nil {
		return err
	}
	log.Printf("Creating fixtLogFile %v", fixtLogPath)
	fixtLogFile, err := os.Create(fixtLogPath)
	if err != nil {
		log.Panicf("creating log file: %v", err)
	}
	defer func() {
		if err := fixtLogFile.Close(); err != nil {
			panic(err)
		}
	}()

	// Get client to run a remote fixture.
	rfcl, err := cl.RunFixture(ctx)
	if err != nil {
		panic(err)
	}

	config := &ibundle.RunFixtureConfig{
		TestVars:       cfg.testVars,
		DataDir:        "/tmp/dat",
		OutDir:         filepath.Join(cfg.localOutDir, "fixtures"), // ??
		TempDir:        "/tmp",
		Target:         cfg.Target,
		KeyFile:        cfg.KeyFile,
		KeyDir:         cfg.KeyDir,
		LocalBundleDir: cfg.localBundleDir,
	}

	var fixtErrs []*ibundle.RunFixtureError

	type fixtMethod int
	const (
		setUp fixtMethod = iota
		tearDown
	)

	var token *ibundle.RunFixtureSetUpDone
	// runFixt runs the method for the current fixture.
	// runFixt leaves logging for the fixture open. Caller is responsible to close the log
	// by calling closeLog if non-nil.
	runFixt := func(method fixtMethod) error {
		if err := cfg.Logger.AddWriter(fixtLogFile, log.LstdFlags); err != nil {
			panic("todo")
		}
		defer func() {
			if err := cfg.Logger.RemoveWriter(fixtLogFile); err != nil {
				panic(err)
			}
		}()

		switch method {
		case setUp:
			if err := rfcl.Send(&ibundle.RunFixtureRequest{
				Control: &ibundle.RunFixtureRequest_SetUp{
					SetUp: &ibundle.RunFixtureSetUpRequest{
						Name:   r,
						Config: config,
					},
				},
			}); err != nil {
				return fmt.Errorf("set up failed: %v", err)
			}
		case tearDown:
			if err := rfcl.Send(&ibundle.RunFixtureRequest{
				Control: &ibundle.RunFixtureRequest_TearDown{
					TearDown: &ibundle.RunFixtureTearDownRequest{
						Token: token,
					},
				},
			}); err != nil {
				return fmt.Errorf("tear down failed: %v", err)
			}
		}

		for {
			msg, err := rfcl.Recv()
			if err == io.EOF {
				// TearDown completed.
				if method != tearDown {
					return fmt.Errorf("connection has been closed while tear down method is not called")
				}
				// TODO: copy files under fixture's OutDir to fixtResDir.
				return nil
			} else if err != nil {
				return fmt.Errorf("rfcl.Recv(): %v", err)
			}
			timestamp, err := ptypes.Timestamp(msg.Timestamp)
			if err != nil {
				return fmt.Errorf("ptypes.Timestamp(%v): %v", msg.Timestamp, err)
			}
			ts := timestamp.Format(testOutputTimeFmt)

			// Handle messages from the runner.
			switch v := msg.Control.(type) {
			case *ibundle.RunFixtureResponse_Error:
				fixtErrs = append(fixtErrs, v.Error)

				cfg.Logger.Logf("[%s] Error at %s:%d: %s", ts, filepath.Base(v.Error.File), v.Error.Line, v.Error.Reason)
				if v.Error.Stack != "" {
					cfg.Logger.Logf("[%s] Stack trace:\n%s", ts, v.Error.Stack)
				}
			case *ibundle.RunFixtureResponse_Log:
				cfg.Logger.Logf("[%s] %s", ts, v.Log)
			case *ibundle.RunFixtureResponse_SetUpDone:
				// Set up is done. We receive this message only when the method is set up.
				if method != setUp {
					return fmt.Errorf("received SetUpDone when set up method is not called")
				}
				token = v.SetUpDone
				return nil
			}
			// TODO: Consider adding fixture end message. The message can have timing log
			// to import with st.Import.
		}
	}

	if r != "" {
		if err := runFixt(setUp); err != nil {
			return err
		}
	}

	var fixtErrStr []string
	for _, e := range fixtErrs {
		fixtErrStr = append(fixtErrStr, fmt.Sprintf("fixture %s has failed: %v", r, e.Reason))
	}

	if err := runTests(fixtErrStr); err != nil {
		return fmt.Errorf("runTests(): %v", err)
	}

	if r != "" {
		if err := runFixt(tearDown); err != nil {
			return err
		}
	}
	// fixtErrStr is not used. Fixture errors are reported separately.
	return nil
}

// runLocalTests executes tests as described by cfg on hst and returns the results.
// It is only used for RunTestsMode.
func runLocalTests(ctx context.Context, cfg *Config) ([]*EntityResult, error) {
	cfg.Logger.Status("Running local tests on target")
	ctx, st := timing.Start(ctx, "run_local_tests")
	defer st.End()

	hst, err := connectToTarget(ctx, cfg)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to %s", cfg.Target)
	}

	// runMatchingTests runs local tests matching the given pattern, and returns the results of the
	// run. fixtErrs are non-nil if fixture failures have happened.
	runMatchingTests := func(ctx context.Context, patterns []string, startFixtureName string, fixtErrs []string) ([]*EntityResult, error) {
		runTests := func(ctx context.Context, patterns []string) (results []*EntityResult, unstarted []string, err error) {
			return runLocalTestsOnce(ctx, cfg, hst, patterns, startFixtureName, fixtErrs)
		}
		beforeRetry := func(ctx context.Context) bool {
			oldHst := hst
			var connErr error
			if hst, connErr = connectToTarget(ctx, cfg); connErr != nil {
				cfg.Logger.Log("Failed reconnecting to target: ", connErr)
				return false
			}
			// The ephemeral devserver uses the SSH connection to the DUT, so a new devserver needs to be
			// created if a new SSH connection was established.
			if hst != oldHst {
				if cfg.ephemeralDevserver != nil {
					if devErr := startEphemeralDevserver(ctx, hst, cfg); devErr != nil {
						cfg.Logger.Log("Failed restarting ephemeral devserver: ", connErr)
						return false
					}
				}
				if cfg.tlwServerForDUT != "" {
					f, err := hst.ForwardRemoteToLocal("tcp", "127.0.0.1:0", cfg.tlwServer, func(e error) {
						cfg.Logger.Logf("remote forwarder error: %s", e)
					})
					if err != nil {
						cfg.Logger.Log("Failed reconnecting remote port forwarding: ", err)
						return false
					}
					cfg.tlwServerForDUT = f.ListenAddr().String()

				}
			}
			return true
		}
		return runTestsWithRetry(ctx, cfg, patterns, runTests, beforeRetry)
	}

	cl, clean, err := createFixtureClient(ctx, cfg.remoteBundleDir)
	if err != nil {
		return nil, err
	}
	if clean != nil {
		defer clean()
	}

	categorizeLocalTests, err := createLocalTestCategorizer(ctx, cfg, hst)
	if err != nil {
		log.Panicf("createLocalTestCategorizer: %v", err)
	}

	// List up local tests matching cfg.Patterns before running tests to categorize them.
	lts, err := listLocalTests(ctx, cfg, hst)
	if err != nil {
		panic(err)
	}
	// TODO: Update listLocalTests to return []*testing.EntityInfo.
	lpts := make([]*testing.EntityInfo, len(lts), len(lts))
	for i, t := range lts {
		lpts[i] = &t
	}
	b2rf2ts := categorizeLocalTests(lpts)

	// Start running local tests.
	start := time.Now()

	var entityResults []*EntityResult
	for b, r2ts := range b2rf2ts {
		cfg.Logger.Logf("Running tests in %v", b)

		for r, ts := range r2ts {
			pats := make([]string, len(ts), len(ts))
			for i, t := range ts {
				pats[i] = t.Name
			}

			// Here, we run fixtures and tests.
			if err := runFixtureAndTests(ctx, cfg, cl, r, func(fixtErrs []string) error {
				res, err := runMatchingTests(ctx, pats, r, fixtErrs)
				if err != nil{ 1
					return fmt.Errorf("runMatchingTests(%#v, %#v): %v", pats, fixtErrs, err)
				}
				entityResults = append(entityResults, res...)
				return nil
			}); err != nil {
				panic(err)
			}
		}
	}

	elapsed := time.Since(start)
	cfg.Logger.Logf("Ran %v local test(s) in %v", len(entityResults), elapsed.Round(time.Millisecond))

	return entityResults, nil
}

type localRunnerHandle struct {
	cmd            *ssh.Cmd
	stdout, stderr io.Reader
}

// Close kills and waits the remote process.
func (h *localRunnerHandle) Close(ctx context.Context) error {
	h.cmd.Abort()
	return h.cmd.Wait(ctx)
}

// startLocalRunner asynchronously starts local_test_runner on hst and passes args to it.
// args.FillDeprecated() is called first to backfill any deprecated fields for old runners.
// The caller is responsible for reading the handle's stdout and closing the handle.
func startLocalRunner(ctx context.Context, cfg *Config, hst *ssh.Conn, args *runner.Args) (*localRunnerHandle, error) {
	args.FillDeprecated()
	argsData, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("marshal args: %v", err)
	}

	cmd := localRunnerCommand(ctx, cfg, hst)
	cmd.cmd.Stdin = bytes.NewBuffer(argsData)
	stdout, err := cmd.cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open stdout pipe: %v", err)
	}
	stderr, err := cmd.cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open stderr pipe: %v", err)
	}

	if err := cmd.cmd.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start local_test_runner: %v", err)
	}
	return &localRunnerHandle{cmd.cmd, stdout, stderr}, nil
}

// runLocalTestsOnce synchronously runs local_test_runner to run local tests
// matching the supplied patterns (rather than cfg.Patterns).
//
// Results from started tests and the names of tests that should have been
// started but weren't (in the order in which they should've been run) are
// returned.
func runLocalTestsOnce(ctx context.Context, cfg *Config, hst *ssh.Conn, patterns []string, startFixtureName string, setUpErrs []string) (
	results []*EntityResult, unstarted []string, err error) {
	ctx, st := timing.Start(ctx, "run_local_tests_once")
	defer st.End()

	// Older local_test_runner does not create the specified output directory.
	// TODO(crbug.com/1000549): Delete this workaround after 20191001.
	// This workaround costs one round-trip time to the DUT.
	if err := hst.Command("mkdir", "-p", cfg.localOutDir).Run(ctx); err != nil {
		return nil, nil, err
	}
	args := runner.Args{
		Mode: runner.RunTestsMode,
		RunTests: &runner.RunTestsArgs{
			BundleArgs: bundle.RunTestsArgs{
				Patterns:          patterns,
				TestVars:          cfg.testVars,
				DataDir:           cfg.localDataDir,
				OutDir:            cfg.localOutDir,
				Devservers:        cfg.devservers,
				TLWServer:         cfg.tlwServerForDUT,
				DUTName:           cfg.Target,
				WaitUntilReady:    cfg.waitUntilReady,
				HeartbeatInterval: heartbeatInterval,
				BuildArtifactsURL: cfg.buildArtifactsURL,
				DownloadMode:      cfg.downloadMode,
				StartFixtureName:  startFixtureName,
				SetUpErrors:       setUpErrs,
			},
			BundleGlob: cfg.localBundleGlob(),
			Devservers: cfg.devservers,
		},
	}
	setRunnerTestDepsArgs(cfg, &args)

	handle, err := startLocalRunner(ctx, cfg, hst, &args)
	if err != nil {
		return nil, nil, err
	}
	defer handle.Close(ctx)

	// Read stderr in the background so it can be included in error messages.
	stderrReader := newFirstLineReader(handle.stderr)

	crf := func(src, dst string) error {
		return moveFromHost(ctx, cfg, hst, src, dst)
	}
	df := func(ctx context.Context, outDir string) string {
		return diagnoseLocalRunError(ctx, cfg, outDir)
	}
	results, unstarted, rerr := readTestOutput(ctx, cfg, handle.stdout, crf, df)

	// Check that the runner exits successfully first so that we don't give a useless error
	// about incorrectly-formed output instead of e.g. an error about the runner being missing.
	timeout := defaultLocalRunnerWaitTimeout
	if cfg.localRunnerWaitTimeout > 0 {
		timeout = cfg.localRunnerWaitTimeout
	}
	wctx, wcancel := context.WithTimeout(ctx, timeout)
	defer wcancel()
	if err := handle.cmd.Wait(wctx); err != nil {
		return results, unstarted, stderrReader.appendToError(err, stderrTimeout)
	}
	return results, unstarted, rerr
}

// formatBytes formats bytes as a human-friendly string.
func formatBytes(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
	)
	switch {
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float32(bytes)/float32(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float32(bytes)/float32(kb))
	}
	return fmt.Sprintf("%d B", bytes)
}

// startEphemeralDevserver starts an ephemeral devserver serving on hst.
// cfg's ephemeralDevserver and devservers fields are updated.
// If ephemeralDevserver is non-nil, it is closed first.
func startEphemeralDevserver(ctx context.Context, hst *ssh.Conn, cfg *Config) error {
	closeEphemeralDevserver(ctx, cfg) // ignore errors; this may rely on a now-dead SSH connection

	lis, err := hst.ListenTCP(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: ephemeralDevserverPort})
	if err != nil {
		return fmt.Errorf("failed to reverse-forward a port: %v", err)
	}

	cacheDir := filepath.Join(cfg.tastDir, "devserver", "static")
	es, err := newEphemeralDevserver(lis, cacheDir, cfg.extraAllowedBuckets)
	if err != nil {
		return err
	}

	cfg.ephemeralDevserver = es
	cfg.devservers = []string{fmt.Sprintf("http://%s", lis.Addr())}
	return nil
}

// closeEphemeralDevserver closes and resets cfg.ephemeralDevserver if non-nil.
func closeEphemeralDevserver(ctx context.Context, cfg *Config) error {
	var err error
	if cfg.ephemeralDevserver != nil {
		err = cfg.ephemeralDevserver.Close(ctx)
		cfg.ephemeralDevserver = nil
	}
	return err
}

// diagnoseLocalRunError is used to attempt to diagnose the cause of an error encountered
// while running local tests. It returns a string that can be returned by a diagnoseRunErrorFunc.
// Files useful for diagnosis might be saved under outDir.
func diagnoseLocalRunError(ctx context.Context, cfg *Config, outDir string) string {
	if cfg.hst == nil || ctxutil.DeadlineBefore(ctx, time.Now().Add(sshPingTimeout)) {
		return ""
	}
	if err := cfg.hst.Ping(ctx, sshPingTimeout); err == nil {
		return ""
	}
	return "Lost SSH connection: " + diagnoseSSHDrop(ctx, cfg, outDir)
}
