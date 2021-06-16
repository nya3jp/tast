// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runtest

import (
	"context"
	"time"

	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
)

// envConfig contains configurations of the testing environment.
type envConfig struct {
	BootID                 func() (bootID string, err error)
	ExtraSSHHandlers       []SSHHandler
	GetDUTInfo             func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error)
	GetSysInfoState        func(req *protocol.GetSysInfoStateRequest) (*protocol.GetSysInfoStateResponse, error)
	CollectSysInfo         func(req *protocol.CollectSysInfoRequest) (*protocol.CollectSysInfoResponse, error)
	DownloadPrivateBundles func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error)
	OnRunLocalTestsInit    func(init *protocol.RunTestsInit)
	OnRunRemoteTestsInit   func(init *protocol.RunTestsInit)
	LocalBundles           []*testing.Registry
	RemoteBundles          []*testing.Registry
}

// EnvOption can be passed to SetUp to customize the testing environment.
type EnvOption func(*envConfig)

// WithBootID specifies a function called to compute a boot ID.
func WithBootID(f func() (bootID string, err error)) EnvOption {
	return func(cfg *envConfig) {
		cfg.BootID = f
	}
}

// WithExtraSSHHandlers specifies extra SSH handlers installed to a fake SSH
// server.
func WithExtraSSHHandlers(handlers []SSHHandler) EnvOption {
	return func(cfg *envConfig) {
		cfg.ExtraSSHHandlers = handlers
	}
}

// WithGetDUTInfo specifies a function that implements GetDUTInfo handler.
func WithGetDUTInfo(f func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error)) EnvOption {
	return func(cfg *envConfig) {
		cfg.GetDUTInfo = f
	}
}

// WithGetSysInfoState specifies a function that implements GetSysInfoState
// handler.
func WithGetSysInfoState(f func(req *protocol.GetSysInfoStateRequest) (*protocol.GetSysInfoStateResponse, error)) EnvOption {
	return func(cfg *envConfig) {
		cfg.GetSysInfoState = f
	}
}

// WithCollectSysInfo specifies a function that implements CollectSysInfo
// handler.
func WithCollectSysInfo(f func(req *protocol.CollectSysInfoRequest) (*protocol.CollectSysInfoResponse, error)) EnvOption {
	return func(cfg *envConfig) {
		cfg.CollectSysInfo = f
	}
}

// WithDownloadPrivateBundles specifies a function that implements
// DownloadPrivateBundles handler.
func WithDownloadPrivateBundles(f func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error)) EnvOption {
	return func(cfg *envConfig) {
		cfg.DownloadPrivateBundles = f
	}
}

// WithOnRunLocalTestsInit specifies a function that is called on the beginning
// of RunTests for the local test runner.
func WithOnRunLocalTestsInit(f func(init *protocol.RunTestsInit)) EnvOption {
	return func(cfg *envConfig) {
		cfg.OnRunLocalTestsInit = f
	}
}

// WithOnRunRemoteTestsInit specifies a function that is called on the beginning
// of RunTests for the remote test runner.
func WithOnRunRemoteTestsInit(f func(init *protocol.RunTestsInit)) EnvOption {
	return func(cfg *envConfig) {
		cfg.OnRunRemoteTestsInit = f
	}
}

// WithLocalBundles specifies fake local test bundles to be installed.
func WithLocalBundles(bs ...*testing.Registry) EnvOption {
	return func(cfg *envConfig) {
		cfg.LocalBundles = bs
	}
}

// WithRemoteBundles specifies fake remote test bundles to be installed.
func WithRemoteBundles(bs ...*testing.Registry) EnvOption {
	return func(cfg *envConfig) {
		cfg.RemoteBundles = bs
	}
}

// defaultEnvConfig returns envConfig that is used by default.
func defaultEnvConfig() *envConfig {
	const defaultBundleName = "bundle"
	localReg := testing.NewRegistry(defaultBundleName)
	localReg.AddTestInstance(&testing.TestInstance{
		Name:    "example.Local",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	})
	remoteReg := testing.NewRegistry(defaultBundleName)
	remoteReg.AddTestInstance(&testing.TestInstance{
		Name:    "example.Remote",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	})

	return &envConfig{
		BootID: func() (bootID string, err error) {
			return "01234567-89ab-cdef-0123-456789abcdef", nil
		},
		GetDUTInfo: func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error) {
			return &protocol.GetDUTInfoResponse{
				DutInfo: &protocol.DUTInfo{
					Features: &protocol.DUTFeatures{
						Software: &protocol.SoftwareFeatures{
							// We must report non-empty features. Otherwise Tast CLI considers the response
							// as an error.
							// TODO(b/187793617): Remove this once we fully migrate to the gRPC protocol and
							// GetDUTInfo gets capable of returning errors.
							Available: []string{"mock"},
						},
					},
					OsVersion: "Mock OS v3.1415926535",
				},
			}, nil
		},
		GetSysInfoState: func(req *protocol.GetSysInfoStateRequest) (*protocol.GetSysInfoStateResponse, error) {
			return &protocol.GetSysInfoStateResponse{}, nil
		},
		CollectSysInfo: func(req *protocol.CollectSysInfoRequest) (*protocol.CollectSysInfoResponse, error) {
			return &protocol.CollectSysInfoResponse{}, nil
		},
		DownloadPrivateBundles: func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error) {
			return &protocol.DownloadPrivateBundlesResponse{}, nil
		},
		OnRunLocalTestsInit:  func(init *protocol.RunTestsInit) {},
		OnRunRemoteTestsInit: func(init *protocol.RunTestsInit) {},
		LocalBundles:         []*testing.Registry{localReg},
		RemoteBundles:        []*testing.Registry{remoteReg},
	}
}
