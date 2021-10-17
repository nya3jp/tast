// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package olddriver

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	gotesting "testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/runtest"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
)

func TestRemoteRun(t *gotesting.T) {
	const (
		localTestName  = "pkg.LocalTest"
		remoteTestName = "pkg.RemoteTest"
	)

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	localReg := testing.NewRegistry("bundle")
	localReg.AddTestInstance(&testing.TestInstance{
		Name:    localTestName,
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
	})
	remoteReg := testing.NewRegistry("bundle")
	remoteReg.AddTestInstance(&testing.TestInstance{
		Name:    remoteTestName,
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
	})

	var gotInit *protocol.RunTestsInit
	var gotBundleConfig *protocol.BundleConfig
	env := runtest.SetUp(t,
		runtest.WithLocalBundles(localReg),
		runtest.WithRemoteBundles(remoteReg),
		runtest.WithOnRunRemoteTestsInit(func(init *protocol.RunTestsInit, bcfg *protocol.BundleConfig) {
			gotInit = init
			gotBundleConfig = bcfg
		}),
	)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.BuildArtifactsURL = "gs://foo/bar"
		// Use IPv4 loopback address with invalid port numbers so that they
		// never resolve to valid destination.
		cfg.Devservers = []string{"http://127.0.0.1:11111111", "http://127.0.0.1:22222222"}
		cfg.TestVars = map[string]string{"abc": "def"}
		cfg.MaybeMissingVars = "xyz"
	})
	state := env.State()
	state.RemoteDevservers = []string{"http://127.0.0.1:33333333", "http://127.0.0.1:44444444"}
	dutInfo := &protocol.DUTInfo{
		Features: &protocol.DUTFeatures{
			Software: &protocol.SoftwareFeatures{
				Available:   []string{"dep1", "dep2"},
				Unavailable: []string{"dep3"},
			},
			Hardware: &protocol.HardwareFeatures{},
		},
	}

	results, err := RunRemoteTests(ctx, cfg, state, dutInfo, cfg.Target())
	if err != nil {
		t.Errorf("RunRemoteTests failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("RunRemoteTests returned %v result(s); want 1", len(results))
	} else if results[0].Name != remoteTestName {
		t.Errorf("RunRemoteTests returned result for test %q; want %q", results[0].Name, remoteTestName)
	}

	wantBundleConfig := &protocol.BundleConfig{
		PrimaryTarget: &protocol.TargetDevice{
			BundleDir: cfg.LocalBundleDir(),
			DutConfig: &protocol.DUTConfig{
				SshConfig: &protocol.SSHConfig{
					ConnectionSpec: cfg.Target(),
					KeyFile:        cfg.KeyFile(),
					KeyDir:         cfg.KeyDir(),
				},
				TlwName: cfg.Target(),
			},
		},
		CompanionDuts: map[string]*protocol.DUTConfig{},
		MetaTestConfig: &protocol.MetaTestConfig{
			TastPath: exe,
			RunFlags: []string{
				"-build=" + strconv.FormatBool(cfg.Build()),
				"-keyfile=" + cfg.KeyFile(),
				"-keydir=" + cfg.KeyDir(),
				"-remoterunner=" + cfg.RemoteRunner(),
				"-remotebundledir=" + cfg.RemoteBundleDir(),
				"-remotedatadir=" + cfg.RemoteDataDir(),
				"-localrunner=" + cfg.LocalRunner(),
				"-localbundledir=" + cfg.LocalBundleDir(),
				"-localdatadir=" + cfg.LocalDataDir(),
				"-devservers=" + strings.Join(cfg.Devservers(), ","),
				"-buildartifactsurl=" + cfg.BuildArtifactsURL(),
			},
		},
	}
	if diff := cmp.Diff(gotBundleConfig, wantBundleConfig, protocmp.Transform()); diff != "" {
		t.Errorf("BundleConfig message mismatch (-got +want):\n%s", diff)
	}

	wantInit := &protocol.RunTestsInit{
		RunConfig: &protocol.RunConfig{
			Tests: nil,
			Features: &protocol.Features{
				CheckDeps: true,
				Dut:       dutInfo.Features,
				Infra: &protocol.InfraFeatures{
					Vars:             cfg.TestVars(),
					MaybeMissingVars: cfg.MaybeMissingVars(),
				},
			},
			Dirs: &protocol.RunDirectories{
				DataDir: cfg.RemoteDataDir(),
				OutDir:  cfg.RemoteOutDir(),
				TempDir: "",
			},
			ServiceConfig: &protocol.ServiceConfig{
				Devservers:  state.RemoteDevservers,
				TlwServer:   "",
				TlwSelfName: "",
			},
			DataFileConfig: &protocol.DataFileConfig{
				BuildArtifactsUrl: cfg.BuildArtifactsURL(),
				DownloadMode:      protocol.DownloadMode_BATCH,
			},
			StartFixtureState: &protocol.StartFixtureState{},
			HeartbeatInterval: ptypes.DurationProto(time.Second),
			WaitUntilReady:    false,
		},
	}
	if diff := cmp.Diff(gotInit, wantInit, protocmp.Transform()); diff != "" {
		t.Errorf("RunTestsInit message mismatch (-got +want):\n%s", diff)
	}
}

func TestRemoteRunCopyOutput(t *gotesting.T) {
	const (
		testName = "pkg.Test"
		outFile  = "somefile.txt"
		outData  = "somedata"
	)

	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(&testing.TestInstance{
		Name:    testName,
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			if err := ioutil.WriteFile(filepath.Join(s.OutDir(), outFile), []byte(outData), 0644); err != nil {
				s.Fatal("WriteFile failed: ", err)
			}
		},
	})

	env := runtest.SetUp(t,
		runtest.WithLocalBundles(testing.NewRegistry("bundle")),
		runtest.WithRemoteBundles(reg),
	)
	ctx := env.Context()
	cfg := env.Config(nil)
	state := env.State()

	if _, err := RunRemoteTests(ctx, cfg, state, nil, cfg.Target()); err != nil {
		t.Fatalf("RunRemoteTests failed: %v", err)
	}

	out, err := ioutil.ReadFile(filepath.Join(cfg.ResDir(), testLogsDir, testName, outFile))
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}
	if string(out) != outData {
		t.Errorf("%s was corrupted: got %q, want %q", outFile, string(out), outData)
	}
}

// TestRemoteMaxFailures makes sure that RunRemoteTests does not run any tests if maximum failures allowed has been reach.
func TestRemoteMaxFailures(t *gotesting.T) {
	reg := testing.NewRegistry("bundle")
	for _, testName := range []string{"t1", "t2"} {
		reg.AddTestInstance(&testing.TestInstance{
			Name:    testName,
			Timeout: time.Minute,
			Func: func(ctx context.Context, s *testing.State) {
				s.Error("Failed")
			},
		})
	}

	env := runtest.SetUp(t,
		runtest.WithLocalBundles(testing.NewRegistry("bundle")),
		runtest.WithRemoteBundles(reg),
	)
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.MaxTestFailures = 1
	})
	state := env.State()

	results, err := RunRemoteTests(ctx, cfg, state, nil, cfg.Target())
	if err == nil {
		t.Error("RunRemoteTests passed unexpectedly")
	}
	if len(results) != 1 {
		t.Errorf("RunRemoteTests return %v results; want 1", len(results))
	}
}

// TODO(derat): Add a test that verifies that GetInitialSysInfo is called before tests are run.
// Also verify that state.StartedRun is false if we see an early failure during GetInitialSysInfo.
