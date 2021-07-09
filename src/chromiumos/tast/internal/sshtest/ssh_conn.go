// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sshtest

import (
	"context"
	"crypto/rsa"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"chromiumos/tast/shutil"
	"chromiumos/tast/ssh"
)

var staticUserKey, staticHostKey *rsa.PrivateKey
var onceGenerateStaticKeys sync.Once

func staticKeys() (userKey, hostKey *rsa.PrivateKey) {
	onceGenerateStaticKeys.Do(func() {
		staticUserKey, staticHostKey = MustGenerateKeys()
	})
	return staticUserKey, staticHostKey
}

// ConnectToServer establishes a connection to srv using key.
// base is used as a base set of options.
func ConnectToServer(ctx context.Context, srv *SSHServer, key *rsa.PrivateKey, base *ssh.Options) (*ssh.Conn, error) {
	keyFile, err := WriteKey(key)
	if err != nil {
		return nil, err
	}
	defer os.Remove(keyFile)

	o := *base
	o.KeyFile = keyFile
	if err = ssh.ParseTarget(srv.Addr().String(), &o); err != nil {
		return nil, err
	}
	s, err := ssh.New(ctx, &o)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// TimeoutType describes different types of timeouts that can be simulated during SSH "exec" requests.
type TimeoutType int

const (
	// NoTimeout indicates that TestData.Ctx shouldn't be canceled.
	NoTimeout TimeoutType = iota
	// StartTimeout indicates that TestData.Ctx should be canceled before the command starts.
	StartTimeout
	// EndTimeout indicates that TestData.Ctx should be canceled after the command runs but before its status is returned.
	EndTimeout
)

// TestDataConn wraps data common to all tests.
// Whereas TastData only manages SSHServer it additionally owns connection to the server.
type TestDataConn struct {
	Srv *SSHServer // local SSH server
	// Hst is a connection to srv.
	Hst *ssh.Conn

	// Ctx is used for performaing operations using Hst.
	Ctx context.Context
	// Cancel cancels Ctx to simulate a timeout.
	Cancel func()

	// ExecTimeout directs how "exec" requests should time out.
	ExecTimeout TimeoutType
}

// NewTestDataConn sets up local SSH server and connection to it, and
// returns them together as a TestDataConn struct.
// Caller must call Close after use.
func NewTestDataConn(t *testing.T) *TestDataConn {
	td := &TestDataConn{}
	td.Ctx, td.Cancel = context.WithCancel(context.Background())

	userKey, hostKey := staticKeys()

	var err error
	if td.Srv, err = NewSSHServer(&userKey.PublicKey, hostKey, td.handleExec); err != nil {
		t.Fatal(err)
	}

	if td.Hst, err = ConnectToServer(td.Ctx, td.Srv, userKey, &ssh.Options{}); err != nil {
		td.Srv.Close()
		t.Fatal(err)
	}

	return td
}

// Close releases resources associated with td.
func (td *TestDataConn) Close() {
	td.Srv.Close()
	td.Hst.Close(td.Ctx)
	td.Cancel()
}

// handleExec handles an SSH "exec" request sent to td.Srv by executing the requested command.
func (td *TestDataConn) handleExec(req *ExecReq) {
	// PutFiles sends multiple "exec" requests.
	// Ignore its initial "sha1sum" so we can hang during the tar command instead.
	ignoreTimeout := strings.HasPrefix(req.Cmd, "sha1sum ")

	// If a timeout was requested, cancel the context and then sleep for an arbitrary-but-long
	// amount of time to make sure that the client sees the expired context before the command
	// actually runs.
	if td.ExecTimeout == StartTimeout && !ignoreTimeout {
		td.Cancel()
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

	if td.ExecTimeout == EndTimeout && !ignoreTimeout {
		td.Cancel()
		time.Sleep(time.Minute)
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
