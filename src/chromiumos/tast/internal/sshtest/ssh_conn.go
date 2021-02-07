// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sshtest

import (
	"context"
	"crypto/rsa"
	"fmt"
	"os"
	"testing"
	"time"

	"chromiumos/tast/shutil"
	"chromiumos/tast/ssh"
)

var userKey, hostKey *rsa.PrivateKey

func init() {
	userKey, hostKey = MustGenerateKeys()
}

// ConnectToServer establishes a connection to target using key.
// base is used as a base set of options.
func ConnectToServer(ctx context.Context, target string, key *rsa.PrivateKey, base *ssh.Options) (*ssh.Conn, error) {
	keyFile, err := WriteKey(key)
	if err != nil {
		return nil, err
	}
	defer os.Remove(keyFile)

	o := *base
	o.KeyFile = keyFile
	if err = ssh.ParseTarget(target, &o); err != nil {
		return nil, err
	}
	s, err := ssh.New(ctx, &o)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// TestDataConn wraps data common to all tests.
// Whereas TastData only manages SSHServer it additionally owns connection to the server.
type TestDataConn struct {
	Srv *Server // local SSH server
	// Hst is a connection to Srv.
	Hst *ssh.Conn

	// Ctx is used for performaing operations using Hst.
	Ctx context.Context
	// cancel cancels Ctx to simulate a timeout.
	cancel func()

	beforeStart handleHook
	beforeEnd   handleHook
}

type handleHook func(cmd string)

type testDataConnOpt func(*TestDataConn)

// WithBeforeStartHook returns an option to register a hook called before the
// server sends a reply to the request reporting the start of the command.
func WithBeforeStartHook(f handleHook) testDataConnOpt {
	return func(c *TestDataConn) {
		c.beforeStart = f
	}
}

// WithBeforeEndtHook returns an option to register a hook called before the
// server returns the exit status and closes the channel.
func WithBeforeEndHook(f handleHook) testDataConnOpt {
	return func(c *TestDataConn) {
		c.beforeEnd = f
	}
}

// NewTestDataConn sets up local SSH server and connection to it, and
// returns them together as a TestDataConn struct.
// Caller must call Close after use.
func NewTestDataConn(t *testing.T, opt ...testDataConnOpt) *TestDataConn {
	td := &TestDataConn{}
	for _, o := range opt {
		o(td)
	}

	td.Ctx, td.cancel = context.WithCancel(context.Background())

	var err error
	if td.Srv, err = NewServer(&userKey.PublicKey, hostKey, td.handleExec); err != nil {
		t.Fatal(err)
	}

	if td.Hst, err = ConnectToServer(context.Background(), td.Srv.Addr().String(), userKey, &ssh.Options{}); err != nil {
		td.Srv.Close()
		t.Fatal(err)
	}

	// Automatically abort the test if it takes too long time.
	go func() {
		const timeout = 10 * time.Second
		select {
		case <-td.Ctx.Done():
			return
		case <-time.After(timeout):
		}
		t.Errorf("Test blocked for %v", timeout)
		td.cancel()
	}()

	return td
}

// Close releases resources associated with td.
func (td *TestDataConn) Close() {
	td.Srv.Close()
	td.Hst.Close(td.Ctx)
	td.cancel()
}

// handleExec handles an SSH "exec" request sent to td.Srv by executing the requested command.
func (td *TestDataConn) handleExec(req *ExecReq) {
	if td.beforeStart != nil {
		td.beforeStart(req.Cmd)
	}
	req.Start(true)

	var status int
	switch req.Cmd {
	case shellCmd("", []string{"long_sleep"}):
		time.Sleep(time.Hour)
	default:
		status = req.RunRealCmd()
	}

	if td.beforeEnd != nil {
		td.beforeEnd(req.Cmd)
	}
	req.End(status)
}

// shellCmd builds a shell command string to execute a process with exec.
// It's copied from ssh/platform.go. TODO(oka): consider refactoring if duplication becomes bigger.
func shellCmd(dir string, args []string) string {
	cmd := "exec " + shutil.EscapeSlice(args)
	if dir != "" {
		// Return 125 (chosen arbitrarily) if dir does not exist.
		// TODO(nya): Consider handling the directory error more gracefully.
		cmd = fmt.Sprintf("cd %s > /dev/null 2>&1 || exit 125; %s", shutil.Escape(dir), cmd)
	}
	return cmd
}
