// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"bytes"
	"context"
	"crypto/rsa"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"chromiumos/tast/cmd/logging"
	"chromiumos/tast/common/control"
	"chromiumos/tast/common/host/test"

	"github.com/google/subcommands"
)

const (
	keyBits = 1024
)

var (
	userKey, hostKey *rsa.PrivateKey
)

func init() {
	var err error
	if userKey, hostKey, err = test.GenerateKeys(keyBits); err != nil {
		panic(err)
	}
}

// localTestData holds data shared between tests that exercise the Local function.
type localTestData struct {
	srv    *test.SSHServer
	logbuf bytes.Buffer
	cfg    Config
}

// newLocalTestData performs setup for tests that exercise the Local function.
// The localTestData struct that it returns is always non-nil, and tests should call
// its close method even if an error is returned.
func newLocalTestData() (td *localTestData, err error) {
	td = &localTestData{}
	if td.srv, err = test.NewSSHServer(&userKey.PublicKey, hostKey); err != nil {
		return td, err
	}
	if td.cfg.KeyFile, err = test.WriteKey(userKey); err != nil {
		return td, err
	}
	if td.cfg.ResDir, err = ioutil.TempDir("", "local_test."); err != nil {
		return td, err
	}
	td.cfg.Logger = logging.NewSimple(&td.logbuf, log.LstdFlags, true)
	td.cfg.Target = td.srv.Addr().String()

	return td, nil
}

func (td *localTestData) close() {
	if td.srv != nil {
		td.srv.Close()
	}
	if td.cfg.KeyFile != "" {
		os.Remove(td.cfg.KeyFile)
	}
	if td.cfg.ResDir != "" {
		os.RemoveAll(td.cfg.ResDir)
	}
}

func TestLocalSuccess(t *testing.T) {
	td, err := newLocalTestData()
	defer td.close()
	if err != nil {
		t.Fatal(err)
	}

	ob := bytes.Buffer{}
	mw := control.NewMessageWriter(&ob)
	mw.WriteMessage(&control.RunStart{time.Unix(1, 0), 0})
	mw.WriteMessage(&control.RunEnd{time.Unix(2, 0), "", ""})
	td.srv.FakeCmd(fmt.Sprintf("%s -report -datadir=%s", localTestsBuiltinPath, localDataBuiltinDir),
		0, ob.Bytes(), []byte{})

	if status := Local(context.Background(), &td.cfg); status != subcommands.ExitSuccess {
		t.Errorf("Local() = %v; want %v (%v)", status, subcommands.ExitSuccess, td.logbuf.String())
	}
}

func TestLocalExecFailure(t *testing.T) {
	td, err := newLocalTestData()
	defer td.close()
	if err != nil {
		t.Fatal(err)
	}

	ob := bytes.Buffer{}
	mw := control.NewMessageWriter(&ob)
	mw.WriteMessage(&control.RunStart{time.Unix(1, 0), 0})
	mw.WriteMessage(&control.RunEnd{time.Unix(2, 0), "", ""})
	const stderr = "some failure message\n"
	td.srv.FakeCmd(fmt.Sprintf("%s -report -datadir=%s", localTestsBuiltinPath, localDataBuiltinDir),
		1, ob.Bytes(), []byte(stderr))

	if status := Local(context.Background(), &td.cfg); status != subcommands.ExitFailure {
		t.Errorf("Local() = %v; want %v", status, subcommands.ExitFailure)
	}
	if !strings.Contains(td.logbuf.String(), stderr) {
		t.Errorf("Local() logged %q; want substring %q", td.logbuf.String(), stderr)
	}
}
