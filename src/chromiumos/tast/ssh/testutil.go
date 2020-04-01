// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import (
	"context"
	"crypto/rsa"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"chromiumos/tast/internal/sshtest"
)

var userKey, hostKey *rsa.PrivateKey

func init() {
	userKey, hostKey = sshtest.MustGenerateKeys()
}

// connectToServer establishes a connection to srv using key.
// base is used as a base set of options.
func connectToServer(ctx context.Context, srv *sshtest.SSHServer, key *rsa.PrivateKey, base *Options) (*Conn, error) {
	keyFile, err := sshtest.WriteKey(key)
	if err != nil {
		return nil, err
	}
	defer os.Remove(keyFile)

	o := *base
	o.KeyFile = keyFile
	if err = ParseTarget(srv.Addr().String(), &o); err != nil {
		return nil, err
	}
	s, err := New(ctx, &o)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// TestRequestTimeoutType describes different types of timeouts that can be simulated during SSH "exec" requests.
// TODO(oka): rename this type and constants to indicate they are for test only.
type TestRequestTimeoutType int

const (
	// TestRequestNoTimeout indicates that TestData.Ctx shouldn't be canceled.
	TestRequestNoTimeout TestRequestTimeoutType = iota
	// TestRequestStartTimeout indicates that TestData.Ctx should be canceled before the command starts.
	TestRequestStartTimeout
	// TestRequestEndTimeout indicates that TestData.Ctx should be canceled after the command runs but before its status is returned.
	TestRequestEndTimeout
)

// TestData wraps data common to all tests.
type TestData struct {
	srv *sshtest.SSHServer // local SSH server
	// Hst is a connection to srv.
	Hst *Conn

	// Ctx is used for performaing operations using Hst.
	Ctx    context.Context
	cancel func() // cancels Ctx to simulate a timeout

	nextCmd string // next command to be executed by client
	// ExecTimeout directs how "exec" requests should time out.
	ExecTimeout TestRequestTimeoutType
}

// NewTestData sets up local SSH server and connection to it, and
// returns them together as a testData struct.
// Caller must call Close after use.
func NewTestData(t *testing.T) *TestData {
	td := &TestData{}
	td.Ctx, td.cancel = context.WithCancel(context.Background())

	var err error
	if td.srv, err = sshtest.NewSSHServer(&userKey.PublicKey, hostKey, td.handleExec); err != nil {
		t.Fatal(err)
	}

	if td.Hst, err = connectToServer(td.Ctx, td.srv, userKey, &Options{}); err != nil {
		td.srv.Close()
		t.Fatal(err)
	}
	td.Hst.AnnounceCmd = func(cmd string) { td.nextCmd = cmd }

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
func (td *TestData) Close() {
	td.srv.Close()
	td.Hst.Close(td.Ctx)
	td.cancel()
}

// handleExec handles an SSH "exec" request sent to td.srv by executing the requested command.
// The command must already be present in td.nextCmd.
func (td *TestData) handleExec(req *sshtest.ExecReq) {
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
	if td.ExecTimeout == TestRequestStartTimeout && !ignoreTimeout {
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

	if td.ExecTimeout == TestRequestEndTimeout && !ignoreTimeout {
		td.cancel()
		time.Sleep(time.Minute)
	}
	req.End(status)
}
