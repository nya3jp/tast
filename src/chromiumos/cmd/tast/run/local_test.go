// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"strings"
	gotesting "testing"
	"time"

	"chromiumos/cmd/tast/logging"
	"chromiumos/tast/control"
	"chromiumos/tast/host/test"
	"chromiumos/tast/testing"

	"github.com/google/subcommands"
)

var userKey, hostKey *rsa.PrivateKey

func init() {
	userKey, hostKey = test.MustGenerateKeys()
}

// localTestData holds data shared between tests that exercise the Local function.
type localTestData struct {
	srvData *test.TestData
	logbuf  bytes.Buffer
	cfg     Config
}

// newLocalTestData performs setup for tests that exercise the Local function.
// Panics on error.
func newLocalTestData() *localTestData {
	td := localTestData{srvData: test.NewTestData(userKey, hostKey)}
	td.cfg.KeyFile = td.srvData.UserKeyFile

	var err error
	if td.cfg.ResDir, err = ioutil.TempDir("", "local_test."); err != nil {
		td.srvData.Close()
		panic(err)
	}
	td.cfg.Logger = logging.NewSimple(&td.logbuf, log.LstdFlags, true)
	td.cfg.Target = td.srvData.Srv.Addr().String()

	return &td
}

func (td *localTestData) close() {
	td.srvData.Close()
	os.RemoveAll(td.cfg.ResDir)
}

func addCheckBundleFakeCmd(srv *test.SSHServer, status int) {
	srv.FakeCmd(fmt.Sprintf("test -d '%s'", localBundleBuiltinDir), status, []byte{}, []byte{})
}

func TestLocalSuccess(t *gotesting.T) {
	td := newLocalTestData()
	defer td.close()

	addCheckBundleFakeCmd(td.srvData.Srv, 0)

	ob := bytes.Buffer{}
	mw := control.NewMessageWriter(&ob)
	mw.WriteMessage(&control.RunStart{time.Unix(1, 0), 0})
	mw.WriteMessage(&control.RunEnd{time.Unix(2, 0), "", "", ""})
	cmd := fmt.Sprintf("%s -bundles='%s/*' -report -datadir=%s",
		localRunnerPath, localBundleBuiltinDir, localDataBuiltinDir)
	td.srvData.Srv.FakeCmd(cmd, 0, ob.Bytes(), []byte{})

	if status, _ := Local(context.Background(), &td.cfg); status != subcommands.ExitSuccess {
		t.Errorf("Local() = %v; want %v (%v)", status, subcommands.ExitSuccess, td.logbuf.String())
	}
}

// TODO(derat): Delete this after 20180524: https://crbug.com/809185
func TestLocalSuccessOldPaths(t *gotesting.T) {
	td := newLocalTestData()
	defer td.close()

	// If the check reports that the new bundle path doesn't exist, Local should fall
	// back to the old bundle and data paths.
	addCheckBundleFakeCmd(td.srvData.Srv, 1)

	ob := bytes.Buffer{}
	mw := control.NewMessageWriter(&ob)
	mw.WriteMessage(&control.RunStart{time.Unix(1, 0), 0})
	mw.WriteMessage(&control.RunEnd{time.Unix(2, 0), "", "", ""})
	cmd := fmt.Sprintf("%s -bundles='%s/*' -report -datadir=%s",
		localRunnerPath, localBundleOldBuiltinDir, localDataOldBuiltinDir)
	td.srvData.Srv.FakeCmd(cmd, 0, ob.Bytes(), []byte{})

	if status, _ := Local(context.Background(), &td.cfg); status != subcommands.ExitSuccess {
		t.Errorf("Local() = %v; want %v (%v)", status, subcommands.ExitSuccess, td.logbuf.String())
	}
}

func TestLocalExecFailure(t *gotesting.T) {
	td := newLocalTestData()
	defer td.close()

	addCheckBundleFakeCmd(td.srvData.Srv, 0)

	ob := bytes.Buffer{}
	mw := control.NewMessageWriter(&ob)
	mw.WriteMessage(&control.RunStart{time.Unix(1, 0), 0})
	mw.WriteMessage(&control.RunEnd{time.Unix(2, 0), "", "", ""})
	const stderr = "some failure message\n"
	cmd := fmt.Sprintf("%s -bundles='%s/*' -report -datadir=%s",
		localRunnerPath, localBundleBuiltinDir, localDataBuiltinDir)
	td.srvData.Srv.FakeCmd(cmd, 1, ob.Bytes(), []byte(stderr))

	if status, _ := Local(context.Background(), &td.cfg); status != subcommands.ExitFailure {
		t.Errorf("Local() = %v; want %v", status, subcommands.ExitFailure)
	}
	if !strings.Contains(td.logbuf.String(), stderr) {
		t.Errorf("Local() logged %q; want substring %q", td.logbuf.String(), stderr)
	}
}

func TestLocalPrint(t *gotesting.T) {
	td := newLocalTestData()
	defer td.close()

	addCheckBundleFakeCmd(td.srvData.Srv, 0)

	tests := []testing.Test{
		testing.Test{Name: "pkg.Test", Desc: "This is a test", Attr: []string{"attr1", "attr2"}},
		testing.Test{Name: "pkg.AnotherTest", Desc: "Another test"},
	}
	b, err := json.Marshal(tests)
	if err != nil {
		t.Fatal(err)
	}
	cmd := fmt.Sprintf("%s -bundles='%s/*' -listtests", localRunnerPath, localBundleBuiltinDir)
	td.srvData.Srv.FakeCmd(cmd, 0, b, []byte{})

	// Verify one-name-per-line output.
	out := bytes.Buffer{}
	td.cfg.PrintDest = &out
	td.cfg.PrintMode = PrintNames
	if status, _ := Local(context.Background(), &td.cfg); status != subcommands.ExitSuccess {
		t.Errorf("Local() = %v; want %v (%v)", status, subcommands.ExitSuccess, td.logbuf.String())
	}
	if exp := fmt.Sprintf("%s\n%s\n", tests[0].Name, tests[1].Name); out.String() != exp {
		t.Errorf("Local() printed %q; want %q", out.String(), exp)
	}

	// Verify JSON output.
	out.Reset()
	td.logbuf.Reset()
	td.cfg.PrintMode = PrintJSON
	if status, _ := Local(context.Background(), &td.cfg); status != subcommands.ExitSuccess {
		t.Errorf("Local() = %v; want %v (%v)", status, subcommands.ExitSuccess, td.logbuf.String())
	}
	outTests := make([]testing.Test, 0)
	if err = json.Unmarshal(out.Bytes(), &outTests); err != nil {
		t.Error("Failed to unmarshal output from Local(): ", err)
	}
	if !reflect.DeepEqual(outTests, tests) {
		t.Errorf("Local() printed tests %v; want %v", outTests, tests)
	}
}
