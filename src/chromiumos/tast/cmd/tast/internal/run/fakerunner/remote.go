// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fakerunner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	gotesting "testing"

	"chromiumos/tast/cmd/tast/internal/logging"
	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/testutil"
)

const (
	// FakeRunnerArgsFile is a file containing args written when acting as fake runner.
	FakeRunnerArgsFile = "fake_runner_args.json"

	fakeRunnerName       = "fake_runner"             // symlink to this executable created by NewRemoteTestData
	fakeRunnerConfigFile = "fake_runner_config.json" // config file read when acting as fake runner
)

func init() {
	// If the binary was executed via a symlink created by NewRemoteTestData,
	// behave like a test runner instead of running unit tests.
	if filepath.Base(os.Args[0]) == fakeRunnerName {
		os.Exit(runFakeRunner())
	}
}

// fakeRunnerConfig describes this executable's output when it's acting as a fake test runner.
type fakeRunnerConfig struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	Status int    `json:"status"`
}

// runFakeRunner saves stdin to the current directory, reads a config, and writes the requested
// data to stdout and stderr. It returns the status code to exit with.
func runFakeRunner() int {
	dir := filepath.Dir(os.Args[0])

	// Write the arguments we received.
	af, err := os.Create(filepath.Join(dir, FakeRunnerArgsFile))
	if err != nil {
		log.Fatal(err)
	}
	defer af.Close()
	if _, err = io.Copy(af, os.Stdin); err != nil {
		log.Fatal(err)
	}

	if err = json.NewEncoder(af).Encode(os.Args[1:]); err != nil {
		log.Fatal(err)
	}

	// Read our configuration.
	cf, err := os.Open(filepath.Join(dir, fakeRunnerConfigFile))
	if err != nil {
		log.Fatal(err)
	}
	defer cf.Close()

	cfg := fakeRunnerConfig{}
	if err = json.NewDecoder(cf).Decode(&cfg); err != nil {
		log.Fatal(err)
	}

	os.Stdout.Write([]byte(cfg.Stdout))
	os.Stderr.Write([]byte(cfg.Stderr))
	return cfg.Status
}

// RemoteTestData holds data corresponding to the current unit test.
type RemoteTestData struct {
	Dir    string                  // temp dir
	LogBuf bytes.Buffer            // logging output
	Cfg    config.Config           // config passed to remote
	State  config.State            // state passed to remote
	Args   jsonprotocol.RunnerArgs // args that were passed to fake runner
}

type fakeRemoteRunnerData struct {
	stdout string
	stderr string
	status int
}

func (rd *fakeRemoteRunnerData) setUp(dir string) (string, error) {
	exec, err := os.Executable()
	if err != nil {
		return "", err
	}

	// Create a symlink to ourselves that can be executed as a fake test runner.
	runner := filepath.Join(dir, fakeRunnerName)
	if err = os.Symlink(exec, runner); err != nil {
		return "", err
	}

	// Write a config file telling the fake runner what to do.
	rcfg := fakeRunnerConfig{Stdout: rd.stdout, Stderr: rd.stderr, Status: rd.status}
	f, err := os.Create(filepath.Join(dir, fakeRunnerConfigFile))
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err = json.NewEncoder(f).Encode(&rcfg); err != nil {
		return "", err
	}
	return runner, nil
}

// NewRemoteTestData creates a temporary directory with a symlink back to the unit test binary
// that's currently running. It also writes a config file instructing the test binary about
// its stdout, stderr, and status code when running as a fake runner.
func NewRemoteTestData(t *gotesting.T, stdout, stderr string, status int) *RemoteTestData {
	td := RemoteTestData{}
	td.Dir = testutil.TempDir(t)
	td.Cfg.Logger = logging.NewSimple(&td.LogBuf, true, true)
	td.Cfg.ResDir = filepath.Join(td.Dir, "results")
	if err := os.MkdirAll(td.Cfg.ResDir, 0755); err != nil {
		os.RemoveAll(td.Dir)
		t.Fatal(err)
	}
	td.Cfg.Devservers = MockDevservers
	td.Cfg.BuildArtifactsURL = MockBuildArtifactsURL
	td.Cfg.DownloadMode = planner.DownloadLazy
	td.Cfg.LocalRunner = MockLocalRunner
	td.Cfg.LocalBundleDir = MockLocalBundleDir
	td.Cfg.LocalDataDir = MockLocalDataDir
	td.Cfg.RemoteOutDir = filepath.Join(td.Cfg.ResDir, "out.tmp")
	td.Cfg.TotalShards = 1
	td.Cfg.ShardIndex = 0

	path, err := (&fakeRemoteRunnerData{stdout, stderr, status}).setUp(td.Dir)
	if err != nil {
		os.RemoveAll(td.Dir)
		t.Fatal(err)
	}
	td.Cfg.RemoteRunner = path

	td.State.RemoteDevservers = td.Cfg.Devservers

	return &td
}

// Close removes the temporary directory.
func (td *RemoteTestData) Close() {
	td.State.Close(context.Background())
	os.RemoveAll(td.Dir)
}
