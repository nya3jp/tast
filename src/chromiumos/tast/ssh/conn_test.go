// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"chromiumos/tast/internal/sshtest"
	"chromiumos/tast/ssh"
	"chromiumos/tast/testutil"
)

var userKey, hostKey = sshtest.MustGenerateKeys()

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
	if hst, err := sshtest.ConnectToServer(ctx, srv, userKey, &ssh.Options{ConnectRetries: 1}); err == nil {
		t.Error("Unexpectedly able to connect to server with inadequate retries")
		hst.Close(ctx)
	}

	// With two retries (i.e. three attempts), the connection should be successfully established.
	srv.RejectConns(2)
	if hst, err := sshtest.ConnectToServer(ctx, srv, userKey, &ssh.Options{ConnectRetries: 2}); err != nil {
		t.Error("Failed connecting to server despite adequate retries: ", err)
	} else {
		hst.Close(ctx)
	}
}

func TestPing(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	td.Srv.AnswerPings(true)
	if err := td.Hst.Ping(td.Ctx, time.Minute); err != nil {
		t.Errorf("Got error when pinging host: %v", err)
	}

	td.Srv.AnswerPings(false)
	if err := td.Hst.Ping(td.Ctx, time.Millisecond); err == nil {
		t.Errorf("Didn't get expected error when pinging host with short timeout")
	}

	// Cancel the context to simulate it having expired.
	td.Cancel()
	if err := td.Hst.Ping(td.Ctx, time.Minute); err == nil {
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

	opt := ssh.Options{KeyDir: td}
	if err = ssh.ParseTarget(srv.Addr().String(), &opt); err != nil {
		t.Fatal(err)
	}
	hst, err := ssh.New(context.Background(), &opt)
	if err != nil {
		t.Fatal(err)
	}
	hst.Close(context.Background())
}
func TestGenerateRemoteAddress(t *testing.T) {
	t.Parallel()
	srv, err := sshtest.NewSSHServer(&userKey.PublicKey, hostKey, func(*sshtest.ExecReq) {})
	if err != nil {
		t.Fatal("Failed starting server: ", err)
	}
	defer srv.Close()

	ctx := context.Background()
	hst, err := sshtest.ConnectToServer(ctx, srv, userKey, &ssh.Options{})
	if err != nil {
		t.Fatal("Unexpectedly unable to connect to server: ", err)
	}
	defer hst.Close(ctx)

	got, err := hst.GenerateRemoteAddress(2345)
	want := "127.0.0.1:2345"
	if got != want {
		t.Fatalf("hst.GenerateRemoteAddress(2345) = %q, want: %q", got, want)
	}
}
