// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package run starts test runners and interprets their output.
package run

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strconv"

	"github.com/google/subcommands"
	"go.chromium.org/chromiumos/config/go/api/test/tls"
	"google.golang.org/grpc"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/devserver"
	"chromiumos/tast/cmd/tast/internal/run/prepare"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/cmd/tast/internal/run/runnerclient"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/sshconfig"
	"chromiumos/tast/ssh"
)

// Status describes the result of a Run call.
type Status struct {
	// ExitCode contains the exit code that should be used by the tast process.
	ExitCode subcommands.ExitStatus
	// ErrorMsg describes the reason why the run failed.
	ErrorMsg string
	// FailedBeforeRun is true if a failure occurred before trying to run tests,
	// e.g. while compiling tests. If so, the caller shouldn't write a results dir.
	FailedBeforeRun bool
}

// successStatus describes a successful run.
var successStatus = Status{}

// errorStatusf returns a Status describing a failing run. format and args are combined to produce the error
// message, which is both logged to cfg.Logger and included in the returned status.
func errorStatusf(cfg *config.Config, code subcommands.ExitStatus, format string, args ...interface{}) Status {
	msg := fmt.Sprintf(format, args...)
	cfg.Logger.Log(msg)
	return Status{ExitCode: code, ErrorMsg: msg}
}

// Run executes or lists tests per cfg and returns the results.
// Messages are logged using cfg.Logger as the run progresses.
// If an error is encountered, status.ErrorMsg will be logged to cfg.Logger before returning,
// but the caller may wish to log it again later to increase its prominence if additional messages are logged.
func Run(ctx context.Context, cfg *config.Config, state *config.State, cc *target.ConnCache) (status Status, results []*resultsjson.Result) {
	defer func() {
		// If we didn't get to the point where we started trying to run tests,
		// report that to the caller so they can avoid writing a useless results dir.
		if status.ExitCode == subcommands.ExitFailure && !state.StartedRun {
			status.FailedBeforeRun = true
		}
	}()

	conn, err := cc.Conn(ctx)
	if err != nil {
		return errorStatusf(cfg, subcommands.ExitFailure, "Failed to connect to %s: %v", cfg.Target, err), nil
	}

	if state.ReportsConn != nil {
		cl := protocol.NewReportsClient(state.ReportsConn)
		state.ReportsClient = cl
		strm, err := cl.LogStream(ctx)
		if err != nil {
			return errorStatusf(cfg, subcommands.ExitFailure, "Failed to start LogStream streaming RPC: %v", err), nil
		}
		defer strm.CloseAndRecv()
		state.ReportsLogStream = strm
	}

	// Always start an ephemeral devserver for remote tests if TLWServer is not specified.
	if cfg.TLWServer == "" && cfg.RunRemote {
		es, err := startEphemeralDevserverForRemoteTests(ctx, cfg, state)
		if err != nil {
			return errorStatusf(cfg, subcommands.ExitFailure, "Failed to start ephemeral devserver for remote tests: %v", err), nil
		}
		defer es.Close(ctx)
	}

	if err := prepare.Prepare(ctx, cfg, state, conn); err != nil {
		return errorStatusf(cfg, subcommands.ExitFailure, "Failed to build and push: %v", err), nil
	}

	switch cfg.Mode {
	case config.ListTestsMode:
		results, err := listTests(ctx, cfg, state, cc)
		if err != nil {
			return errorStatusf(cfg, subcommands.ExitFailure, "Failed to list tests: %v", err), nil
		}
		return successStatus, results
	case config.RunTestsMode:
		results, err := runTests(ctx, cfg, state, cc)
		if err != nil {
			return errorStatusf(cfg, subcommands.ExitFailure, "Failed to run tests: %v", err), results
		}
		return successStatus, results
	default:
		return errorStatusf(cfg, subcommands.ExitFailure, "Unhandled mode %d", cfg.Mode), nil
	}
}

// startEphemeralDevserverForRemoteTests starts an ephemeral devserver for remote tests.
func startEphemeralDevserverForRemoteTests(ctx context.Context, cfg *config.Config, state *config.State) (*devserver.Ephemeral, error) {
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("failed to listen to a local port: %v", err)
	}

	cacheDir := filepath.Join(cfg.TastDir, "devserver", "static")
	es, err := devserver.NewEphemeral(lis, cacheDir, cfg.ExtraAllowedBuckets)
	if err != nil {
		return nil, err
	}

	state.RemoteDevservers = []string{fmt.Sprintf("http://%s", lis.Addr())}
	cfg.Logger.Log("Starting ephemeral devserver at ", state.RemoteDevservers[0], " for remote tests")
	return es, nil
}

// listTests returns the whole tests to run.
func listTests(ctx context.Context, cfg *config.Config, state *config.State, cc *target.ConnCache) ([]*resultsjson.Result, error) {
	if err := runnerclient.GetDUTInfo(ctx, cfg, state, cc); err != nil {
		return nil, err
	}
	testsToRun, testsToSkip, _, err := runnerclient.FindTestsForShard(ctx, cfg, state, cc)
	if err != nil {
		return nil, err
	}
	if cfg.ShardIndex == 0 {
		testsToRun = append(testsToRun, testsToSkip...)
	}
	return testsToRun, nil
}

func runTests(ctx context.Context, cfg *config.Config, state *config.State, cc *target.ConnCache) ([]*resultsjson.Result, error) {
	if err := runnerclient.GetDUTInfo(ctx, cfg, state, cc); err != nil {
		return nil, errors.Wrap(err, "failed to get DUT software features")
	}

	if state.OSVersion == "" {
		cfg.Logger.Log("Target version: not available from target")
	} else {
		cfg.Logger.Logf("Target version: %v", state.OSVersion)
	}

	if err := runnerclient.GetInitialSysInfo(ctx, cfg, state, cc); err != nil {
		return nil, errors.Wrap(err, "failed to get initial sysinfo")
	}

	testsToRun, testsToSkip, testsNotInShard, err := runnerclient.FindTestsForShard(ctx, cfg, state, cc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get test patterns for specified shard")
	}

	// We include all tests to be skipped in shard 0
	if cfg.ShardIndex == 0 {
		testsToRun = append(testsToRun, testsToSkip...)
		testsToSkip = nil
	}

	cfg.TestsToRun = testsToRun
	cfg.TestNamesToSkip = nil
	for _, t := range testsToSkip {
		cfg.TestNamesToSkip = append(cfg.TestNamesToSkip, t.Name)
	}
	for _, t := range testsNotInShard {
		cfg.TestNamesToSkip = append(cfg.TestNamesToSkip, t.Name)
	}

	if cfg.TotalShards > 1 {
		cfg.Logger.Logf("Running shard %d/%d (tests %d/%d)", cfg.ShardIndex+1, cfg.TotalShards,
			len(cfg.TestsToRun), len(cfg.TestsToRun)+len(cfg.TestNamesToSkip))
	}
	if len(testsToRun) == 0 {
		// No tests to run.
		return nil, nil
	}

	var results []*resultsjson.Result
	state.StartedRun = true

	if cfg.RunLocal {
		lres, err := runnerclient.RunLocalTests(ctx, cfg, state, cc)
		results = append(results, lres...)
		if err != nil {
			// TODO(derat): While test runners are always supposed to report success even if tests fail,
			// it'd probably be better to run both types here even if one fails.
			return results, err
		}
	}

	if !cfg.RunRemote {
		return results, nil
	}

	// Run remote tests and merge the results.
	rres, err := runnerclient.RunRemoteTests(ctx, cfg, state)
	results = append(results, rres...)
	return results, err
}

// SetupGrpcServices sets up all Grpc Services in the current run.
func SetupGrpcServices(ctx context.Context, cfg *config.Config, state *config.State) error {
	if err := connectToTLW(ctx, cfg, state); err != nil {
		return errors.Wrap(err, "failed to connect to TLW server")
	}
	if err := connectToReports(ctx, cfg, state); err != nil {
		return errors.Wrap(err, "failed to connect to Reports server")
	}
	return nil
}

// ResolveHosts resolve all hosts in the current run.
func ResolveHosts(ctx context.Context, cfg *config.Config, state *config.State) error {
	// Check if host name needs to be resolved.
	cfg.Target = resolveHost(cfg, cfg.Target)
	for role, dut := range cfg.CompanionDUTs {
		cfg.CompanionDUTs[role] = resolveHost(cfg, dut)
	}
	var err error
	if cfg.Target, err = resolveTarget(ctx, state.TLWConn, cfg.Target); err != nil {
		return errors.Wrap(err, "failed to resolve target")
	}
	for role, dut := range cfg.CompanionDUTs {
		if cfg.CompanionDUTs[role], err = resolveTarget(ctx, state.TLWConn, dut); err != nil {
			return errors.Wrapf(err, "failed to resolve companion DUT %v", dut)
		}
	}
	return nil
}

func resolveHost(cfg *config.Config, target string) string {
	alternateTarget, err := sshconfig.ResolveHost(target)
	if err != nil {
		cfg.Logger.Logf("Error in reading SSH configuaration files: %v", err)
		return target
	}
	if alternateTarget != target {
		cfg.Logger.Logf("Using target %v instead of %v to connect according to SSH configuration files",
			alternateTarget, target)
	}
	return alternateTarget
}

// connectToTLW connects to a TLW service if its address is provided, and stores
// the connection to state.TLWConn.
func connectToTLW(ctx context.Context, cfg *config.Config, state *config.State) error {
	if cfg.TLWServer == "" {
		return nil
	}

	conn, err := grpc.DialContext(ctx, cfg.TLWServer, grpc.WithInsecure())
	if err != nil {
		return err
	}
	state.TLWConn = conn
	return nil
}

// connectToReports connects to the Reports server.
func connectToReports(ctx context.Context, cfg *config.Config, state *config.State) error {
	if cfg.ReportsServer == "" {
		return nil
	}
	conn, err := grpc.DialContext(ctx, cfg.ReportsServer, grpc.WithInsecure())
	if err != nil {
		return err
	}
	state.ReportsConn = conn
	return nil
}

// resolveTarget resolves cfg.Target using the TLW service if available.
func resolveTarget(ctx context.Context, tlwConn *grpc.ClientConn, target string) (resolvedTarget string, err error) {
	if tlwConn == nil {
		return target, nil
	}

	var opts ssh.Options
	if err := ssh.ParseTarget(target, &opts); err != nil {
		return target, err
	}
	host, portStr, err := net.SplitHostPort(opts.Hostname)
	if err != nil {
		host = opts.Hostname
		portStr = "22"
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return target, err
	}

	// Use the OpenDutPort API to resolve the target.
	req := &tls.OpenDutPortRequest{Name: host, Port: int32(port)}
	res, err := tls.NewWiringClient(tlwConn).OpenDutPort(ctx, req)
	if err != nil {
		return target, err
	}

	return fmt.Sprintf("%s@%s:%d", opts.User, res.GetAddress(), res.GetPort()), nil
}
