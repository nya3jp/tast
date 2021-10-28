// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"
	"io"
	"os"
	"sort"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"chromiumos/tast/cmd/tast/internal/run/driverdata"
	"chromiumos/tast/cmd/tast/internal/run/genericexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/testing"
)

// GRPCClient is a GRPC-protocol client to test_runner.
type GRPCClient struct {
	cmd        genericexec.Cmd
	params     *protocol.RunnerInitParams
	msgTimeout time.Duration
	hops       int
}

// NewGRPCClient creates a new GRPCClient.
func NewGRPCClient(cmd genericexec.Cmd, params *protocol.RunnerInitParams, msgTimeout time.Duration, hops int) *GRPCClient {
	return &GRPCClient{
		cmd:        cmd,
		params:     params,
		msgTimeout: msgTimeout,
		hops:       hops,
	}
}

// rpcConn represents a gRPC connection to a test runner.
type rpcConn struct {
	proc genericexec.Process
	conn *rpc.GenericClient
}

// Close closes the gRPC connection to the test runner.
func (c *rpcConn) Close(ctx context.Context) error {
	var firstErr error
	if err := c.conn.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := c.proc.Stdin().Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := c.proc.Wait(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// Conn returns the established gRPC connection.
func (c *rpcConn) Conn() *grpc.ClientConn {
	return c.conn.Conn()
}

// dial connects to the test runner and returned an established gRPC connection.
func (c *GRPCClient) dial(ctx context.Context, req *protocol.HandshakeRequest) (_ *rpcConn, retErr error) {
	proc, err := c.cmd.Interact(ctx, []string{"-rpc"})
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			proc.Stdin().Close()
			proc.Wait(ctx)
		}
	}()

	// Pass through stderr.
	go io.Copy(os.Stderr, proc.Stderr())

	opts := []grpc.DialOption{
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:    c.msgTimeout,
			Timeout: 5 * time.Second,
		}),
	}
	conn, err := rpc.NewClient(ctx, proc.Stdout(), proc.Stdin(), req, opts...)
	if err != nil {
		return nil, err
	}

	return &rpcConn{
		proc: proc,
		conn: conn,
	}, nil
}

// GetDUTInfo retrieves various DUT information needed for test execution.
func (c *GRPCClient) GetDUTInfo(ctx context.Context, req *protocol.GetDUTInfoRequest) (res *protocol.GetDUTInfoResponse, retErr error) {
	defer func() {
		if retErr != nil {
			retErr = errors.Wrap(retErr, "getting DUT info")
		}
	}()

	conn, err := c.dial(ctx, &protocol.HandshakeRequest{RunnerInitParams: c.params})
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	cl := protocol.NewTestServiceClient(conn.Conn())
	return cl.GetDUTInfo(ctx, req)
}

// GetSysInfoState collects the sysinfo state of the DUT.
func (c *GRPCClient) GetSysInfoState(ctx context.Context, req *protocol.GetSysInfoStateRequest) (res *protocol.GetSysInfoStateResponse, retErr error) {
	defer func() {
		if retErr != nil {
			retErr = errors.Wrap(retErr, "getting sysinfo state")
		}
	}()

	conn, err := c.dial(ctx, &protocol.HandshakeRequest{RunnerInitParams: c.params})
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	cl := protocol.NewTestServiceClient(conn.Conn())
	return cl.GetSysInfoState(ctx, req)
}

// CollectSysInfo collects the sysinfo, considering diff from the given initial
// sysinfo state.
func (c *GRPCClient) CollectSysInfo(ctx context.Context, req *protocol.CollectSysInfoRequest) (res *protocol.CollectSysInfoResponse, retErr error) {
	defer func() {
		if retErr != nil {
			retErr = errors.Wrap(retErr, "collecting sysinfo")
		}
	}()

	conn, err := c.dial(ctx, &protocol.HandshakeRequest{RunnerInitParams: c.params})
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	cl := protocol.NewTestServiceClient(conn.Conn())
	return cl.CollectSysInfo(ctx, req)
}

// DownloadPrivateBundles downloads and installs a private test bundle archive
// corresponding to the target version, if one has not been installed yet.
func (c *GRPCClient) DownloadPrivateBundles(ctx context.Context, req *protocol.DownloadPrivateBundlesRequest) (retErr error) {
	defer func() {
		if retErr != nil {
			retErr = errors.Wrap(retErr, "downloading private bundles")
		}
	}()

	conn, err := c.dial(ctx, &protocol.HandshakeRequest{RunnerInitParams: c.params})
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	cl := protocol.NewTestServiceClient(conn.Conn())
	_, err = cl.DownloadPrivateBundles(ctx, req)
	return err
}

// ListTests enumerates tests matching patterns.
func (c *GRPCClient) ListTests(ctx context.Context, patterns []string, features *protocol.Features) (tests []*driverdata.BundleEntity, retErr error) {
	defer func() {
		if retErr != nil {
			retErr = errors.Wrap(retErr, "listing tests")
		}
	}()

	matcher, err := testing.NewMatcher(patterns)
	if err != nil {
		return nil, err
	}

	conn, err := c.dial(ctx, &protocol.HandshakeRequest{RunnerInitParams: c.params})
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	cl := protocol.NewTestServiceClient(conn.Conn())

	req := &protocol.ListEntitiesRequest{
		Features: features,
	}
	res, err := cl.ListEntities(ctx, req)
	if err != nil {
		return nil, err
	}

	for _, e := range res.GetEntities() {
		if e.GetEntity().GetType() != protocol.EntityType_TEST {
			continue
		}
		if !matcher.Match(e.GetEntity().GetName(), e.GetEntity().GetAttributes()) {
			continue
		}
		e.Hops = int32(c.hops)
		tests = append(tests, &driverdata.BundleEntity{
			Bundle:   e.GetEntity().GetLegacyData().GetBundle(),
			Resolved: e,
		})
	}
	return tests, nil
}

// ListFixtures enumerates all fixtures.
func (c *GRPCClient) ListFixtures(ctx context.Context) (fixtures []*driverdata.BundleEntity, retErr error) {
	defer func() {
		if retErr != nil {
			retErr = errors.Wrap(retErr, "listing fixtures")
		}
	}()

	conn, err := c.dial(ctx, &protocol.HandshakeRequest{RunnerInitParams: c.params})
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	cl := protocol.NewTestServiceClient(conn.Conn())

	req := &protocol.ListEntitiesRequest{
		// We don't have access to Features here, so pass nil. This is okay
		// since Features is needed only for skip checks for tests.
		Features: nil,
	}
	res, err := cl.ListEntities(ctx, req)
	if err != nil {
		return nil, err
	}

	for _, e := range res.GetEntities() {
		if e.GetEntity().GetType() != protocol.EntityType_FIXTURE {
			continue
		}
		e.Hops = int32(c.hops)
		fixtures = append(fixtures, &driverdata.BundleEntity{
			Bundle:   e.GetEntity().GetLegacyData().GetBundle(),
			Resolved: e,
		})
	}

	sort.Slice(fixtures, func(i, j int) bool {
		a, b := fixtures[i], fixtures[j]
		if a.Bundle != b.Bundle {
			return a.Bundle < b.Bundle
		}
		return a.Resolved.GetEntity().GetName() < b.Resolved.GetEntity().GetName()
	})
	return fixtures, nil
}
