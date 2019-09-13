// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

func TestPipeListener(t *testing.T) {
	const (
		readStr  = "read"
		writeStr = "write"
	)

	r := bytes.NewBufferString(readStr)
	w := &bytes.Buffer{}

	lis := NewPipeListener(r, w)
	defer lis.Close()

	func() {
		conn, err := lis.Accept()
		if err != nil {
			t.Fatal("Accept failed: ", err)
		}
		defer conn.Close()

		if b, err := ioutil.ReadAll(conn); err != nil {
			t.Error("Read failed: ", err)
		} else if s := string(b); s != readStr {
			t.Errorf("Read returned %q; want %q", s, readStr)
		}

		if _, err := conn.Write([]byte(writeStr)); err != nil {
			t.Error("Write failed: ", err)
		}
		if s := w.String(); s != writeStr {
			t.Errorf("Write wrote %q; want %q", s, writeStr)
		}
	}()

	if _, err := lis.Accept(); err != io.EOF {
		t.Errorf("Accept failed: %v; want %v", err, io.EOF)
	}
}

func TestPipeClientConn(t *testing.T) {
	sr, cw := io.Pipe()
	defer sr.Close()
	defer cw.Close()
	cr, sw := io.Pipe()
	defer cr.Close()
	defer sw.Close()

	srv := grpc.NewServer()
	reflection.Register(srv)

	go srv.Serve(NewPipeListener(sr, sw))
	defer srv.Stop()

	conn, err := NewPipeClientConn(context.Background(), cr, cw)
	if err != nil {
		t.Fatal("NewPipeClientConn failed: ", err)
	}
	defer conn.Close()

	cl := grpc_reflection_v1alpha.NewServerReflectionClient(conn)
	st, err := cl.ServerReflectionInfo(context.Background())
	if err != nil {
		t.Fatal("ServerReflectionInfo RPC call failed: ", err)
	}
	if err := st.CloseSend(); err != nil {
		t.Fatal("ServerReflectionInfo RPC call failed on CloseSend: ", err)
	}
	if _, err := st.Recv(); err == nil {
		t.Fatal("ServerReflectionInfo RPC call returned an unexpected reply")
	} else if err != io.EOF {
		t.Fatal("ServerReflectionInfo RPC call failed on Recv: ", err)
	}
}
