// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package linuxssh

import (
	"context"
	"crypto/rsa"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"chromiumos/tast/host"
	"chromiumos/tast/host/test"
	"chromiumos/tast/testutil"
)

var userKey, hostKey *rsa.PrivateKey

func init() {
	userKey, hostKey = test.MustGenerateKeys()
}

type testData struct {
	*test.TestData

	ctx    context.Context
	cancel context.CancelFunc
	hst    *host.SSH

	execTimeout timeoutType // how "exec" requests should time out
}

func newTestData(t *testing.T) *testData {
	td := &testData{}
	td.TestData = test.NewTestData(userKey, hostKey, td.handleExec)

	td.ctx, td.cancel = context.WithCancel(context.Background())

	var err error
	if td.hst, err = connectToServer(td.ctx, td.Srv, td.UserKeyFile); err != nil {
		td.Close()
		t.Fatal(err)
	}

	// Automatically abort the test if it takes too long time.
	go func() {
		const timeout = 5 * time.Second
		select {
		case <-td.ctx.Done():
			return
		case <-time.After(timeout):
		}
		t.Errorf("Test blocked for %v", timeout)
		td.cancel()
	}()

	return td
}

// timeoutType describes different types of timeouts that can be simulated during SSH "exec" requests.
type timeoutType int

const (
	// noTimeout indicates that testData.ctx shouldn't be canceled.
	noTimeout timeoutType = iota
	// startTimeout indicates that testData.ctx should be canceled before the command starts.
	startTimeout
	// endTimeout indicates that testData.ctx should be canceled after the command runs but before its status is returned.
	endTimeout
)

// handleExec handles an SSH "exec" request sent to td.srv by executing the requested command.
// The command must already be present in td.nextCmd.
func (td *testData) handleExec(req *test.ExecReq) {
	// If a timeout was requested, cancel the context and then sleep for an arbitrary-but-long
	// amount of time to make sure that the client sees the expired context before the command
	// actually runs.
	if td.execTimeout == startTimeout {
		td.cancel()
		time.Sleep(time.Minute)
	}
	req.Start(true)

	status := req.RunRealCmd()

	if td.execTimeout == endTimeout {
		td.cancel()
		time.Sleep(time.Minute)
	}
	req.End(status)
}

func (td *testData) close() {
	td.hst.Close(td.ctx)
	td.cancel()
	td.Close()
}

// connectToServer establishes a connection to srv using key.
// base is used as a base set of options.
func connectToServer(ctx context.Context, srv *test.SSHServer, keyFile string) (*host.SSH, error) {
	o := &host.SSHOptions{KeyFile: keyFile}
	o.KeyFile = keyFile
	if err := host.ParseSSHTarget(srv.Addr().String(), o); err != nil {
		return nil, err
	}
	s, err := host.NewSSH(ctx, o)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// initFileTest creates a temporary directory with a subdirectory containing files.
// The temp dir's and subdir's paths are returned.
func initFileTest(t *testing.T, files map[string]string) (tmpDir, srcDir string) {
	tmpDir = testutil.TempDir(t)
	srcDir = filepath.Join(tmpDir, "src")
	if err := os.Mkdir(srcDir, 0755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}
	if err := testutil.WriteFiles(srcDir, files); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}
	return tmpDir, srcDir
}

// checkFile returns an error if p's contents differ from exp.
func checkFile(p string, exp string) error {
	if b, err := ioutil.ReadFile(p); err != nil {
		return fmt.Errorf("failed to read %v after copy: %v", p, err)
	} else if string(b) != exp {
		return fmt.Errorf("expected content %q after copy, got %v", exp, string(b))
	}
	return nil
}

// checkDir returns an error if dir's contents don't match exp, a map from paths to data.
func checkDir(dir string, exp map[string]string) error {
	if act, err := testutil.ReadFiles(dir); err != nil {
		return err
	} else if !reflect.DeepEqual(exp, act) {
		return fmt.Errorf("files not copied correctly: src %v, dst %v", exp, act)
	}
	return nil
}

func TestGetFileRegular(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	files := map[string]string{"file": "foo"}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	srcFile := filepath.Join(srcDir, "file")
	dstFile := filepath.Join(tmpDir, "file.copy")
	if err := GetFile(td.ctx, td.hst, srcFile, dstFile); err != nil {
		t.Fatal(err)
	}
	if err := checkFile(dstFile, files["file"]); err != nil {
		t.Error(err)
	}

	// GetFile should overwrite local files.
	if err := GetFile(td.ctx, td.hst, srcFile, dstFile); err != nil {
		t.Error(err)
	}
}

func TestGetFileDir(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	files := map[string]string{
		"myfile":     "some data",
		"mydir/file": "this is in a subdirectory",
	}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	// Copy the full source directory.
	dstDir := filepath.Join(tmpDir, "dst")
	if err := GetFile(td.ctx, td.hst, srcDir, dstDir); err != nil {
		t.Fatal(err)
	}
	if err := checkDir(dstDir, files); err != nil {
		t.Error(err)
	}

	// GetFile should overwrite local dirs.
	if err := GetFile(td.ctx, td.hst, srcDir, dstDir); err != nil {
		t.Error(err)
	}
}

func TestGetFileTimeout(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	files := map[string]string{"file": "data"}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	srcFile := filepath.Join(srcDir, "file")
	dstFile := filepath.Join(tmpDir, "file")
	td.execTimeout = startTimeout
	if err := GetFile(td.ctx, td.hst, srcFile, dstFile); err == nil {
		t.Errorf("GetFile() with expired context didn't return error")
	}
}
