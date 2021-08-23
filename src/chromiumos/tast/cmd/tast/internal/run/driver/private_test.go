// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.chromium.org/chromiumos/config/go/api/test/tls"
	"google.golang.org/grpc"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/driver"
	"chromiumos/tast/cmd/tast/internal/run/runtest"
	"chromiumos/tast/internal/devserver"
	"chromiumos/tast/internal/faketlw"
	"chromiumos/tast/internal/protocol"
)

func TestDriver_DownloadPrivateBundles_Disabled(t *testing.T) {
	env := runtest.SetUp(
		t,
		runtest.WithDownloadPrivateBundles(func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error) {
			t.Error("DownloadPrivateBundles called unexpectedly")
			return &protocol.DownloadPrivateBundlesResponse{}, nil
		}),
	)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.DownloadPrivateBundles = false // disable downloading private bundles
		cfg.BuildArtifactsURL = "gs://build-artifacts/foo/bar"
	})

	drv, err := driver.New(ctx, cfg, cfg.Target())
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	if err := drv.DownloadPrivateBundles(ctx); err != nil {
		t.Fatalf("DownloadPrivateBundles failed: %v", err)
	}
}

func TestDriver_DownloadPrivateBundles_Devservers(t *testing.T) {
	const buildArtifactsURL = "gs://build-artifacts/foo/bar"
	devservers := []string{"http://example.com:1111", "http://example.com:2222"}

	called := false
	env := runtest.SetUp(
		t,
		runtest.WithDownloadPrivateBundles(func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error) {
			called = true
			want := &protocol.DownloadPrivateBundlesRequest{
				ServiceConfig: &protocol.ServiceConfig{
					Devservers: devservers,
				},
				BuildArtifactUrl: buildArtifactsURL,
			}
			if diff := cmp.Diff(req, want); diff != "" {
				t.Errorf("DownloadPrivateBundlesRequest mismatch (-got +want):\n%s", diff)
			}
			return &protocol.DownloadPrivateBundlesResponse{}, nil
		}),
	)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.DownloadPrivateBundles = true
		cfg.Devservers = devservers
		cfg.BuildArtifactsURL = buildArtifactsURL
	})

	drv, err := driver.New(ctx, cfg, cfg.Target())
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	if err := drv.DownloadPrivateBundles(ctx); err != nil {
		t.Fatalf("DownloadPrivateBundles failed: %v", err)
	}
	if !called {
		t.Error("DownloadPrivateBundles not called")
	}
}

func TestDriver_DownloadPrivateBundles_EphemeralDevserver(t *testing.T) {
	const buildArtifactsURL = "gs://build-artifacts/foo/bar"

	called := false
	env := runtest.SetUp(
		t,
		runtest.WithDownloadPrivateBundles(func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error) {
			called = true

			if url := req.GetBuildArtifactUrl(); url != buildArtifactsURL {
				t.Errorf("DownloadPrivateBundles: build artifacts URL mismatch: got %q, want %q", url, buildArtifactsURL)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cl := devserver.NewRealClient(ctx, req.GetServiceConfig().GetDevservers(), nil)
			if n := len(cl.UpServerURLs()); n != 1 {
				t.Errorf("DownloadPrivateBundles: Ephemeral devserver is down: got %d devservers, want 1", n)
			}
			return &protocol.DownloadPrivateBundlesResponse{}, nil
		}),
	)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.DownloadPrivateBundles = true
		cfg.UseEphemeralDevserver = true
		cfg.BuildArtifactsURL = buildArtifactsURL
	})

	drv, err := driver.New(ctx, cfg, cfg.Target())
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	if err := drv.DownloadPrivateBundles(ctx); err != nil {
		t.Fatalf("DownloadPrivateBundles failed: %v", err)
	}
	if !called {
		t.Error("DownloadPrivateBundles not called")
	}
}

func TestDriver_DownloadPrivateBundles_TLW(t *testing.T) {
	const (
		tlwSelfName       = "dutname"
		buildArtifactsURL = "gs://build-artifacts/foo/bar"
		archiveURL        = "gs://build-artifacts/foo/bar/tast-bundles.zip"
	)

	stopFunc, tlwAddr := faketlw.StartWiringServer(
		t,
		faketlw.WithDUTName(tlwSelfName),
		faketlw.WithCacheFileMap(map[string][]byte{archiveURL: []byte("abc")}),
	)
	defer stopFunc()

	called := false
	env := runtest.SetUp(
		t,
		runtest.WithDownloadPrivateBundles(func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error) {
			called = true

			if url := req.GetBuildArtifactUrl(); url != buildArtifactsURL {
				t.Errorf("DownloadPrivateBundles: build artifacts URL mismatch: got %q, want %q", url, buildArtifactsURL)
			}

			// We should get tlwDUTName as the TLW name.
			// Due to a bug, an incorrect TLW name is given for now.
			// TODO(b/191318903): Uncomment this check once the bug is fixed.
			// if name := req.GetServiceConfig().GetTlwSelfName(); name != tlwSelfName {
			// 	t.Errorf("DownloadPrivateBundles: TLW name mismatch: got %q, want %q", name, tlwSelfName)
			// }

			// DownloadPrivateBundles is called on the DUT, thus we don't have
			// direct access to the TLW server. Tast CLI should have set up SSH
			// port forwarding.
			if addr := req.GetServiceConfig().GetTlwServer(); addr == tlwAddr {
				t.Errorf("DownloadPrivateBundles: TLW is not port-forwarded (%s)", addr)
			}

			// Make sure TLW is working over the forwarded port.
			conn, err := grpc.Dial(req.GetServiceConfig().GetTlwServer(), grpc.WithInsecure())
			if err != nil {
				t.Errorf("DownloadPrivateBundles: failed to connect to TLW server: %v", err)
				return nil, nil
			}
			defer conn.Close()

			cl := tls.NewWiringClient(conn)
			if _, err := cl.CacheForDut(context.Background(), &tls.CacheForDutRequest{Url: archiveURL, DutName: tlwSelfName}); err != nil {
				t.Errorf("DownloadPrivateBundles: CacheForDut failed: %v", err)
			}
			return &protocol.DownloadPrivateBundlesResponse{}, nil
		}),
	)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.DownloadPrivateBundles = true
		cfg.TLWServer = tlwAddr
		cfg.BuildArtifactsURL = buildArtifactsURL
	})

	drv, err := driver.New(ctx, cfg, cfg.Target())
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	defer drv.Close(ctx)

	if err := drv.DownloadPrivateBundles(ctx); err != nil {
		t.Fatalf("DownloadPrivateBundles failed: %v", err)
	}
	if !called {
		t.Error("DownloadPrivateBundles not called")
	}
}
