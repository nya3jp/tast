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
	"strings"
	"time"

	"go.chromium.org/chromiumos/config/go/api/test/tls"
	"google.golang.org/grpc"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/devserver"
	"chromiumos/tast/cmd/tast/internal/run/prepare"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/cmd/tast/internal/run/runnerclient"
	"chromiumos/tast/cmd/tast/internal/run/sharding"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/errors"
	"chromiumos/tast/framework/protocol"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/sshconfig"
	"chromiumos/tast/internal/xcontext"
	"chromiumos/tast/ssh"
)

const (
	// maxPostReserve is maximum amount of time reserved for post-processing
	// (e.g. writing results and collecting system info).
	maxPostReserve = 15 * time.Second
)

// Run executes or lists tests per cfg and returns the results.
// Messages are logged via ctx as the run progresses.
func Run(ctx context.Context, cfg *config.Config, state *config.State) ([]*resultsjson.Result, error) {
	if err := setUpGRPCServices(ctx, cfg, state); err != nil {
		return nil, errors.Wrap(err, "failed to set up gRPC servers")
	}
	if err := resolveHosts(ctx, cfg, state); err != nil {
		return nil, errors.Wrap(err, "failed to resolve hosts")
	}

	cc := target.NewConnCache(cfg, cfg.Target)
	defer cc.Close(ctx)

	if state.ReportsConn != nil {
		cl := protocol.NewReportsClient(state.ReportsConn)
		state.ReportsClient = cl
		strm, err := cl.LogStream(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to start LogStream streaming RPC")
		}
		defer strm.CloseAndRecv()
		state.ReportsLogStream = strm
	}

	// Always start an ephemeral devserver for remote tests if TLWServer is not specified.
	if cfg.TLWServer == "" {
		es, err := startEphemeralDevserverForRemoteTests(ctx, cfg, state)
		if err != nil {
			return nil, errors.Wrap(err, "failed to start ephemeral devserver for remote tests")
		}
		defer es.Close()
	}

	if err := prepare.Prepare(ctx, cfg, cc); err != nil {
		return nil, errors.Wrap(err, "failed to build and push")
	}

	switch cfg.Mode {
	case config.ListTestsMode:
		results, err := listTests(ctx, cfg, cc)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list tests")
		}
		return results, nil
	case config.RunTestsMode:
		results, err := runTests(ctx, cfg, state, cc)
		if err != nil {
			return results, errors.Wrapf(err, "failed to run tests")
		}
		return results, nil
	default:
		return nil, errors.Errorf("unhandled mode %d", cfg.Mode)
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
	logging.Info(ctx, "Starting ephemeral devserver at ", state.RemoteDevservers[0], " for remote tests")
	return es, nil
}

// listTests returns the whole tests to run.
func listTests(ctx context.Context, cfg *config.Config, cc *target.ConnCache) ([]*resultsjson.Result, error) {
	dutInfo, err := runnerclient.GetDUTInfo(ctx, cfg, cc)
	if err != nil {
		return nil, err
	}

	tests, err := runnerclient.ListTests(ctx, cfg, dutInfo, cc)
	if err != nil {
		return nil, err
	}

	shard := sharding.Compute(tests, cfg.ShardIndex, cfg.TotalShards)

	// Convert protocol.ResolvedEntity to resultsjson.Result.
	results := make([]*resultsjson.Result, len(shard.Included))
	for i, re := range shard.Included {
		test, err := resultsjson.NewTest(re.GetEntity())
		if err != nil {
			return nil, err
		}
		bt := resultsjson.RemoteBundle
		if re.GetHops() > 0 {
			bt = resultsjson.LocalBundle
		}
		results[i] = &resultsjson.Result{
			Test:       *test,
			SkipReason: strings.Join(re.GetSkip().GetReasons(), ", "),
			BundleType: bt,
		}
	}
	return results, nil
}

func runTests(ctx context.Context, cfg *config.Config, state *config.State, cc *target.ConnCache) (results []*resultsjson.Result, retErr error) {
	dutInfo, err := runnerclient.GetDUTInfo(ctx, cfg, cc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get DUT software features")
	}

	if ver := dutInfo.GetOsVersion(); ver == "" {
		logging.Info(ctx, "Target version: not available from target")
	} else {
		logging.Infof(ctx, "Target version: %v", ver)
	}

	initialSysInfo, err := runnerclient.GetInitialSysInfo(ctx, cfg, cc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get initial sysinfo")
	}

	tests, err := runnerclient.ListTests(ctx, cfg, dutInfo, cc)
	if err != nil {
		return nil, err
	}

	shard := sharding.Compute(tests, cfg.ShardIndex, cfg.TotalShards)

	state.TestsToRun = shard.Included
	state.TestNamesToSkip = nil
	for _, t := range shard.Excluded {
		state.TestNamesToSkip = append(state.TestNamesToSkip, t.GetEntity().GetName())
	}

	if cfg.TotalShards > 1 {
		logging.Infof(ctx, "Running shard %d/%d (tests %d/%d)", cfg.ShardIndex+1, cfg.TotalShards,
			len(state.TestsToRun), len(state.TestsToRun)+len(state.TestNamesToSkip))
	}
	if len(state.TestsToRun) == 0 {
		// No tests to run.
		return nil, nil
	}

	// Reserve a bit of time to write results and collect system info.
	// Skip doing this if a very-short timeout was set, since it's confusing
	// to get an immediate timeout in that case.
	postCtx := ctx
	if deadline, ok := ctx.Deadline(); ok {
		postReserve := maxPostReserve
		if time.Until(deadline) < 2*postReserve {
			postReserve = 0
		}
		var cancel xcontext.CancelFunc
		ctx, cancel = xcontext.WithDeadline(ctx, deadline.Add(-postReserve), errors.Errorf("%v: global timeout reached", context.DeadlineExceeded))
		defer cancel(context.Canceled)
	}

	// Write results and collect system info after testing.
	defer func() {
		ctx := postCtx
		if retErr != nil {
			// Print the run error message before moving on to writing results.
			logging.Infof(ctx, "Failed to run tests: %v", retErr)
		}
		complete := retErr == nil
		if err := runnerclient.WriteResults(ctx, cfg, state, results, initialSysInfo, complete, cc); err != nil {
			if retErr == nil {
				retErr = errors.Wrap(err, "failed to write results")
			} else {
				logging.Infof(ctx, "Failed to write results: %v", err)
			}
		}
	}()

	lres, err := runnerclient.RunLocalTests(ctx, cfg, state, dutInfo, cc)
	results = append(results, lres...)
	if err != nil {
		// TODO(derat): While test runners are always supposed to report success even if tests fail,
		// it'd probably be better to run both types here even if one fails.
		return results, err
	}

	rres, err := runnerclient.RunRemoteTests(ctx, cfg, state, dutInfo)
	results = append(results, rres...)
	return results, err
}

// setUpGRPCServices sets up all Grpc Services in the current run.
func setUpGRPCServices(ctx context.Context, cfg *config.Config, state *config.State) error {
	if err := connectToTLW(ctx, cfg, state); err != nil {
		return errors.Wrap(err, "failed to connect to TLW server")
	}
	if err := connectToReports(ctx, cfg, state); err != nil {
		return errors.Wrap(err, "failed to connect to Reports server")
	}
	return nil
}

// resolveHosts resolve all hosts in the current run.
func resolveHosts(ctx context.Context, cfg *config.Config, state *config.State) error {
	// Check if host name needs to be resolved.
	cfg.Target = resolveHost(ctx, cfg.Target)
	for role, dut := range cfg.CompanionDUTs {
		cfg.CompanionDUTs[role] = resolveHost(ctx, dut)
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

func resolveHost(ctx context.Context, target string) string {
	alternateTarget, err := sshconfig.ResolveHost(target)
	if err != nil {
		logging.Infof(ctx, "Error in reading SSH configuaration files: %v", err)
		return target
	}
	if alternateTarget != target {
		logging.Infof(ctx, "Using target %v instead of %v to connect according to SSH configuration files",
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
