// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"google.golang.org/grpc"

	"go.chromium.org/tast/core/cmd/tast/internal/run/driver/internal/drivercore"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/rpc"
	"go.chromium.org/tast/core/internal/run/genericexec"
	"go.chromium.org/tast/core/internal/testing"
)

// Client is a GRPC-protocol client to test_runner.
type Client struct {
	cmd        genericexec.Cmd
	params     *protocol.RunnerInitParams
	msgTimeout time.Duration
	hops       int
}

// New creates a new Client.
func New(cmd genericexec.Cmd, params *protocol.RunnerInitParams, msgTimeout time.Duration, hops int) *Client {
	return &Client{
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
func (c *Client) dial(ctx context.Context, req *protocol.HandshakeRequest) (_ *rpcConn, retErr error) {
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

	// TODO: re-enable keepalive check after finding a proper solution for b/239035591.
	conn, err := rpc.NewClient(ctx, proc.Stdout(), proc.Stdin(), req)
	if err != nil {
		return nil, err
	}

	return &rpcConn{
		proc: proc,
		conn: conn,
	}, nil
}

// GetDUTInfo retrieves various DUT information needed for test execution.
func (c *Client) GetDUTInfo(ctx context.Context, req *protocol.GetDUTInfoRequest) (res *protocol.GetDUTInfoResponse, retErr error) {
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
func (c *Client) GetSysInfoState(ctx context.Context, req *protocol.GetSysInfoStateRequest) (res *protocol.GetSysInfoStateResponse, retErr error) {
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
func (c *Client) CollectSysInfo(ctx context.Context, req *protocol.CollectSysInfoRequest) (res *protocol.CollectSysInfoResponse, retErr error) {
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
func (c *Client) DownloadPrivateBundles(ctx context.Context, req *protocol.DownloadPrivateBundlesRequest) (retErr error) {
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
func (c *Client) ListTests(ctx context.Context, patterns []string, features *protocol.Features) (tests []*drivercore.BundleEntity, retErr error) {
	defer func() {
		if retErr != nil {
			retErr = errors.Wrap(retErr, "listing tests")
		}
	}()

	matcher, err := testing.NewMatcher(patterns)
	if err != nil {
		return nil, err
	}

	logging.Infof(ctx, "Sending ListEntities Request to test runner (hops=%v)", c.hops)
	listEntities := func(timeout time.Duration) (*protocol.ListEntitiesResponse, error) {
		conn, err := c.dial(ctx, &protocol.HandshakeRequest{RunnerInitParams: c.params})
		if err != nil {
			return nil, err
		}
		defer conn.Close(ctx)
		req := &protocol.ListEntitiesRequest{
			Features: features,
		}
		// It should be less than a minute to list entities.
		cl := protocol.NewTestServiceClient(conn.Conn())
		ctxForListEntities, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return cl.ListEntities(ctxForListEntities, req)
	}
	res, err := listEntities(time.Minute)
	if err != nil {
		logging.Infof(ctx, "Failed to send ListEntities request: %v", err)
		logging.Infof(ctx, "Retry sending ListEntities Request to test runner (hops=%v)", c.hops)
		res, err = listEntities(2 * time.Minute)
		if err != nil {
			return nil, err
		}
	}
	logging.Info(ctx, "Got ListEntities Response from local test runner")

	for _, e := range res.GetEntities() {
		if e.GetEntity().GetType() != protocol.EntityType_TEST {
			continue
		}
		if !matcher.Match(e.GetEntity().GetName(), e.GetEntity().GetAttributes()) {
			continue
		}
		e.Hops = int32(c.hops)
		tests = append(tests, &drivercore.BundleEntity{
			Bundle:   e.GetEntity().GetLegacyData().GetBundle(),
			Resolved: e,
		})
	}
	return tests, nil
}

// GlobalRuntimeVars client implementation
func (c *Client) GlobalRuntimeVars(ctx context.Context) (vars []string, retErr error) {
	defer func() {
		if retErr != nil {
			retErr = errors.Wrap(retErr, "listing GlobalRuntimeVars")
		}
	}()

	GlobalRuntimeVars := func() (*protocol.GlobalRuntimeVarsResponse, error) {
		conn, err := c.dial(ctx, &protocol.HandshakeRequest{RunnerInitParams: c.params})
		if err != nil {
			return nil, err
		}
		defer conn.Close(ctx)
		req := &protocol.GlobalRuntimeVarsRequest{}
		// It should be less than a minute to get global vars.
		cl := protocol.NewTestServiceClient(conn.Conn())
		ctxForGlobalRuntimeVars, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		return cl.GlobalRuntimeVars(ctxForGlobalRuntimeVars, req)
	}
	res, err := GlobalRuntimeVars()

	if err != nil {
		logging.Infof(ctx, "Failed to send GlobalRuntimeVars request: %v", err)
		return nil, err
	}
	logging.Info(ctx, "Got GlobalRuntimeVars Response from local test runner")

	var result []string
	for i := 0; i < len(res.Vars); i++ {
		result = append(result, res.Vars[i].Name)
	}
	return result, nil
}

// ListFixtures enumerates all fixtures.
func (c *Client) ListFixtures(ctx context.Context) (fixtures []*drivercore.BundleEntity, retErr error) {
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
		fixtures = append(fixtures, &drivercore.BundleEntity{
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

// StreamFile stream a file from the source file at target DUT to a destination
// file at the host.
func (c *Client) StreamFile(ctx context.Context, src, dest string, offset int64) (nextOffset int64, err error) {
	conn, err := c.dial(ctx, &protocol.HandshakeRequest{RunnerInitParams: c.params})
	if err != nil {
		return offset, errors.Wrapf(err, "failed to establish connection to stream file %s from DUT to %s", src, dest)
	}
	defer conn.Close(ctx)
	cl := protocol.NewTestServiceClient(conn.Conn())
	req := &protocol.StreamFileRequest{Name: src, Offset: offset}
	stream, err := cl.StreamFile(ctx, req)
	if err != nil {
		return offset, errors.Wrapf(err, "failed to stream file %s from DUT to %s", src, dest)
	}
	destDir := filepath.Dir(dest)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return offset, errors.Wrapf(err, "failed to create directory %v for streaming", destDir)
	}
	f, err := os.OpenFile(dest, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return offset, errors.Wrapf(err, "failed to open file %v for streaming", dest)
	}
	defer f.Close()
	nextOffset = offset
	for {
		msg, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				logging.Infof(ctx, "receive EOF while streaming data from file %v", src)
				return nextOffset, nil
			}
			return nextOffset, errors.Wrapf(err, "failed to receive streaming data from file %v", src)
		}
		if _, err := f.Write(msg.GetData()); err != nil {
			return nextOffset, errors.Wrapf(err, "failed to write to streaming file %v", dest)
		}
		nextOffset = msg.GetOffset()
	}
}
