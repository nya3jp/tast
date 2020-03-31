// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package host

import (
	"context"
	"crypto/rsa"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"chromiumos/tast/internal/sshtest"
	"chromiumos/tast/testutil"
)

var userKey, hostKey *rsa.PrivateKey

func init() {
	userKey, hostKey = sshtest.MustGenerateKeys()
}

// connectToServer establishes a connection to srv using key.
// base is used as a base set of options.
func connectToServer(ctx context.Context, srv *sshtest.SSHServer, key *rsa.PrivateKey, base *SSHOptions) (*SSH, error) {
	keyFile, err := sshtest.WriteKey(key)
	if err != nil {
		return nil, err
	}
	defer os.Remove(keyFile)

	o := *base
	o.KeyFile = keyFile
	if err = ParseSSHTarget(srv.Addr().String(), &o); err != nil {
		return nil, err
	}
	s, err := NewSSH(ctx, &o)
	if err != nil {
		return nil, err
	}
	return s, nil
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

// testData wraps data common to all tests.
type testData struct {
	srv *sshtest.SSHServer
	hst *SSH

	ctx    context.Context // used for performing operations using hst
	cancel func()          // cancels ctx to simulate a timeout

	nextCmd     string      // next command to be executed by client
	execTimeout timeoutType // how "exec" requests should time out
}

func newTestData(t *testing.T) *testData {
	td := &testData{}
	td.ctx, td.cancel = context.WithCancel(context.Background())

	var err error
	if td.srv, err = sshtest.NewSSHServer(&userKey.PublicKey, hostKey, td.handleExec); err != nil {
		t.Fatal(err)
	}

	if td.hst, err = connectToServer(td.ctx, td.srv, userKey, &SSHOptions{}); err != nil {
		td.srv.Close()
		t.Fatal(err)
	}
	td.hst.AnnounceCmd = func(cmd string) { td.nextCmd = cmd }

	// Automatically abort the test if it takes too long time.
	go func() {
		const timeout = 10 * time.Second
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

func (td *testData) close() {
	td.srv.Close()
	td.hst.Close(td.ctx)
	td.cancel()
}

// handleExec handles an SSH "exec" request sent to td.srv by executing the requested command.
// The command must already be present in td.nextCmd.
func (td *testData) handleExec(req *sshtest.ExecReq) {
	if req.Cmd != td.nextCmd {
		log.Printf("Unexpected command %q (want %q)", req.Cmd, td.nextCmd)
		req.Start(false)
		return
	}

	// PutFiles sends multiple "exec" requests.
	// Ignore its initial "sha1sum" so we can hang during the tar command instead.
	ignoreTimeout := strings.HasPrefix(req.Cmd, "sha1sum ")

	// If a timeout was requested, cancel the context and then sleep for an arbitrary-but-long
	// amount of time to make sure that the client sees the expired context before the command
	// actually runs.
	if td.execTimeout == startTimeout && !ignoreTimeout {
		td.cancel()
		time.Sleep(time.Minute)
	}
	req.Start(true)

	var status int
	switch req.Cmd {
	case shellCmd("", []string{"long_sleep"}):
		time.Sleep(time.Hour)
	default:
		status = req.RunRealCmd()
	}

	if td.execTimeout == endTimeout && !ignoreTimeout {
		td.cancel()
		time.Sleep(time.Minute)
	}
	req.End(status)
}

func TestRetry(t *testing.T) {
	t.Parallel()
	srv, err := sshtest.NewSSHServer(&userKey.PublicKey, hostKey, func(*sshtest.ExecReq) {})
	if err != nil {
		t.Fatal("Failed starting server: ", err)
	}
	defer srv.Close()

	// Configure the server to reject the next two connections and let the client only retry once.
	srv.RejectConns(2)
	ctx := context.Background()
	if hst, err := connectToServer(ctx, srv, userKey, &SSHOptions{ConnectRetries: 1}); err == nil {
		t.Error("Unexpectedly able to connect to server with inadequate retries")
		hst.Close(ctx)
	}

	// With two retries (i.e. three attempts), the connection should be successfully established.
	srv.RejectConns(2)
	if hst, err := connectToServer(ctx, srv, userKey, &SSHOptions{ConnectRetries: 2}); err != nil {
		t.Error("Failed connecting to server despite adequate retries: ", err)
	} else {
		hst.Close(ctx)
	}
}

func TestPing(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	td.srv.AnswerPings(true)
	if err := td.hst.Ping(td.ctx, time.Minute); err != nil {
		t.Errorf("Got error when pinging host: %v", err)
	}

	td.srv.AnswerPings(false)
	if err := td.hst.Ping(td.ctx, time.Millisecond); err == nil {
		t.Errorf("Didn't get expected error when pinging host with short timeout")
	}

	// Cancel the context to simulate it having expired.
	td.cancel()
	if err := td.hst.Ping(td.ctx, time.Minute); err == nil {
		t.Errorf("Didn't get expected error when pinging host with expired context")
	}
}

func TestKeyDir(t *testing.T) {
	t.Parallel()
	srv, err := sshtest.NewSSHServer(&userKey.PublicKey, hostKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	keyFile, err := sshtest.WriteKey(userKey)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(keyFile)

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	if err = os.Symlink(keyFile, filepath.Join(td, "testing_rsa")); err != nil {
		t.Fatal(err)
	}

	opt := SSHOptions{KeyDir: td}
	if err = ParseSSHTarget(srv.Addr().String(), &opt); err != nil {
		t.Fatal(err)
	}
	hst, err := NewSSH(context.Background(), &opt)
	if err != nil {
		t.Fatal(err)
	}
	hst.Close(context.Background())
}
