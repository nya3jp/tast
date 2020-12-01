// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc"

	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/testutil"
)

func TestFileTransferServerPullDirectory(t *testing.T) {
	// Start a gRPC server.
	gs := grpc.NewServer()
	protocol.RegisterFileTransferServer(gs, newFileTransferServer())

	lis, err := net.ListenTCP("tcp", nil)
	if err != nil {
		t.Fatal("Failed to listen: ", err)
	}

	go gs.Serve(lis)
	defer gs.Stop()

	// Set up a gRPC client.
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatal("Failed to dial: ", err)
	}
	defer conn.Close()

	cl := protocol.NewFileTransferClient(conn)

	// Create a temporary directory holding everything for the test.
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	// Create a source directory containing random files.
	want := map[string]string{
		"a.txt":     "abc",
		"dir/b.txt": "def",
	}
	src := filepath.Join(td, "src")
	if err := testutil.WriteFiles(src, want); err != nil {
		t.Fatal("Failed to set up source dir: ", err)
	}

	// Create an empty destination directory.
	dst := filepath.Join(td, "dst")
	if err := os.Mkdir(dst, 0777); err != nil {
		t.Fatal("Failed to create empty destination dir: ", err)
	}

	// Pull the directory!
	if err := pullDirectory(context.Background(), cl, src, dst); err != nil {
		t.Fatal("Failed to pull directory: ", err)
	}

	// Destination directory should be the same as the former source directory.
	got, err := testutil.ReadFiles(dst)
	if err != nil {
		t.Fatal("Failed to read contents of source dir: ", err)
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("Directory contents mismatch (-got +want):\n%s", diff)
	}

	// Source directory should have been removed.
	if _, err := os.Stat(src); err == nil {
		t.Error("Source directory should have been removed")
	} else if !os.IsNotExist(err) {
		t.Error("Failed to stat source dir: ", err)
	}
}
