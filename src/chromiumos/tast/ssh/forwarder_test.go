// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestForwarder(t *testing.T) {
	local, remote := net.Pipe()
	defer local.Close()

	connFunc := func() (net.Conn, error) { return local, nil }
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to listen a port: %s", err)
	}
	fwd, err := newForwarder(listener, connFunc, nil)
	if err != nil {
		t.Fatal("newForwarder failed:", err)
	}
	t.Log("Forwarder listening at", fwd.ListenAddr())
	defer fwd.Close()

	conn, err := net.Dial("tcp", fwd.ListenAddr().String())
	if err != nil {
		t.Fatal("Dial failed:", err)
	}
	defer conn.Close()

	const (
		sendData = "send data" // sent from local to remote
		recvData = "recv data" // sent from remote to local
	)

	// Start a goroutine that reads sendData and then sends recvData.
	go func() {
		defer remote.Close()

		b := make([]byte, len(sendData))
		if _, err := io.ReadFull(remote, b); err != nil {
			t.Error("Read failed:", err)
			return
		} else if string(b) != sendData {
			t.Errorf("Read %q; want %q", b, sendData)
			return
		}
		if _, err := io.WriteString(remote, recvData); err != nil {
			t.Errorf("Writing %q failed: %v", recvData, err)
			return
		}
	}()

	// In the main goroutine, send sendData and read recvData.
	if _, err := io.WriteString(conn, sendData); err != nil {
		t.Fatalf("Writing %q failed: %v", sendData, err)
	}
	b := make([]byte, len(recvData))
	if _, err := io.ReadFull(conn, b); err != nil {
		t.Fatal("Read failed:", err)
	} else if string(b) != recvData {
		t.Fatalf("Read %q; want %q", b, recvData)
	}
}

func TestForwarderError(t *testing.T) {
	// Copy forwarding errors to a channel.
	ch := make(chan error, 1)
	errFunc := func(err error) { ch <- err }

	// Make the forwarder receive an error when it tries to open the remote connection,
	connErr := errors.New("intentional error")
	connFunc := func() (net.Conn, error) { return nil, connErr }
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to listen a port: %s", err)
	}
	fwd, err := newForwarder(listener, connFunc, errFunc)
	if err != nil {
		t.Fatal("newForwarder failed:", err)
	}
	defer fwd.Close()

	// We should be able to establish a connection to the forwarder's local address,
	// but the error handler should receive the connection error.
	conn, err := net.Dial("tcp", fwd.ListenAddr().String())
	if err != nil {
		t.Fatal("Dial failed:", err)
	}
	defer conn.Close()

	select {
	case err := <-ch:
		if err != connErr {
			t.Fatalf("Got error %q; want %q", err, connErr)
		}
	case <-time.After(time.Minute):
		t.Fatal("Didn't receive any error")
	}
}
