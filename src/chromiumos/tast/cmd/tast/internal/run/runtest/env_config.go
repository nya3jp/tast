// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runtest

import (
	"context"
	"fmt"
	"time"

	"chromiumos/tast/internal/fakesshserver"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
)

// envConfig contains configurations of the testing environment.
type envConfig struct {
	OnRunRemoteTestsInit func(init *protocol.RunTestsInit, bcfg *protocol.BundleConfig)
	LocalBundles         []*testing.Registry
	RemoteBundles        []*testing.Registry

	PrimaryDUT    *dutConfig
	CompanionDUTs map[string]*dutConfig
}

// dutConfig contains configurations of a fake DUT.
type dutConfig struct {
	ExtraSSHHandlers       []fakesshserver.Handler
	BootID                 func() (bootID string, err error)
	GetDUTInfo             func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error)
	GetSysInfoState        func(req *protocol.GetSysInfoStateRequest) (*protocol.GetSysInfoStateResponse, error)
	CollectSysInfo         func(req *protocol.CollectSysInfoRequest) (*protocol.CollectSysInfoResponse, error)
	DownloadPrivateBundles func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error)
	OnRunLocalTestsInit    func(init *protocol.RunTestsInit, bcfg *protocol.BundleConfig)
}

// EnvOption can be passed to SetUp to customize the testing environment.
type EnvOption func(*envConfig)

func (o EnvOption) applyToEnvConfig(cfg *envConfig) { o(cfg) }

// DUTOption can be passed to SetUp to configure the primary DUT, or to
// WithCompanionDUT to configure a companion DUT.
type DUTOption func(*dutConfig)

func (o DUTOption) applyToEnvConfig(cfg *envConfig) { o(cfg.PrimaryDUT) }

// EnvOrDUTOption is an interface satisfied by EnvOption and DUTOption.
type EnvOrDUTOption interface {
	applyToEnvConfig(cfg *envConfig)
}

// WithBootID specifies a function called to compute a boot ID.
func WithBootID(f func() (bootID string, err error)) DUTOption {
	return func(cfg *dutConfig) {
		cfg.BootID = f
	}
}

// WithExtraSSHHandlers specifies extra SSH handlers installed to a fake SSH
// server.
func WithExtraSSHHandlers(handlers []fakesshserver.Handler) DUTOption {
	return func(cfg *dutConfig) {
		cfg.ExtraSSHHandlers = handlers
	}
}

// WithGetDUTInfo specifies a function that implements GetDUTInfo handler.
func WithGetDUTInfo(f func(req *protocol.GetDUTInfoRequest) (*protocol.GetDUTInfoResponse, error)) DUTOption {
	return func(cfg *dutConfig) {
		cfg.GetDUTInfo = f
	}
}

// WithGetSysInfoState specifies a function that implements GetSysInfoState
// handler.
func WithGetSysInfoState(f func(req *protocol.GetSysInfoStateRequest) (*protocol.GetSysInfoStateResponse, error)) DUTOption {
	return func(cfg *dutConfig) {
		cfg.GetSysInfoState = f
	}
}

// WithCollectSysInfo specifies a function that implements CollectSysInfo
// handler.
func WithCollectSysInfo(f func(req *protocol.CollectSysInfoRequest) (*protocol.CollectSysInfoResponse, error)) DUTOption {
	return func(cfg *dutConfig) {
		cfg.CollectSysInfo = f
	}
}

// WithDownloadPrivateBundles specifies a function that implements
// DownloadPrivateBundles handler.
func WithDownloadPrivateBundles(f func(req *protocol.DownloadPrivateBundlesRequest) (*protocol.DownloadPrivateBundlesResponse, error)) DUTOption {
	return func(cfg *dutConfig) {
		cfg.DownloadPrivateBundles = f
	}
}

// WithOnRunLocalTestsInit specifies a function that is called on the beginning
// of RunTests for the local test runner.
func WithOnRunLocalTestsInit(f func(init *protocol.RunTestsInit, bcfg *protocol.BundleConfig)) DUTOption {
	return func(cfg *dutConfig) {
		cfg.OnRunLocalTestsInit = f
	}
}

// WithOnRunRemoteTestsInit specifies a function that is called on the beginning
// of RunTests for the remote test runner.
func WithOnRunRemoteTestsInit(f func(init *protocol.RunTestsInit, bcfg *protocol.BundleConfig)) EnvOption {
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

// WithCompanionDUT adds a companion DUT.
func WithCompanionDUT(role string, opts ...DUTOption) EnvOption {
	return func(cfg *envConfig) {
		if _, ok := cfg.CompanionDUTs[role]; ok {
			panic(fmt.Sprintf("WithCompanionDUT: Duplicated companion DUT role %q", role))
		}
		dcfg := defaultDUTConfig(len(cfg.CompanionDUTs) + 1)
		for _, opt := range opts {
			opt(dcfg)
		}
		cfg.CompanionDUTs[role] = dcfg
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
		OnRunRemoteTestsInit: func(init *protocol.RunTestsInit, bcfg *protocol.BundleConfig) {},
		LocalBundles:         []*testing.Registry{localReg},
		RemoteBundles:        []*testing.Registry{remoteReg},
		PrimaryDUT:           defaultDUTConfig(0),
		CompanionDUTs:        make(map[string]*dutConfig),
	}
}

// defaultDUTConfig returns dutConfig that is used by default.
// dutID should be 0 for the primary DUT, 1+ for companion DUTs.
func defaultDUTConfig(dutID int) *dutConfig {
	return &dutConfig{
		BootID: func() (bootID string, err error) {
			return fmt.Sprintf("bootID-for-server-%d", dutID), nil
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
		OnRunLocalTestsInit: func(init *protocol.RunTestsInit, bcfg *protocol.BundleConfig) {},
	}
}
