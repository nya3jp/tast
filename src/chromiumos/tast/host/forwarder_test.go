// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package host

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestForwarder(t *testing.T) {
	// The remote address here is arbitrary. The test SSH server doesn't support
	// forwarded connections, so we'll swap in a function later that just returns
	// one end of a pipe to simulate creating remote connections.
	fwd, err := NewForwarder(nil, "127.0.0.1:0", "192.0.2.1:12345", nil)
	if err != nil {
		t.Fatal("NewForwarder failed:", err)
	}
	t.Log("Forwarder listening at", fwd.LocalAddr())
	defer fwd.Close()

	local, remote := net.Pipe()
	defer local.Close()
	fwd.connFunc = func() (net.Conn, error) { return local, nil }

	conn, err := net.Dial("tcp", fwd.LocalAddr().String())
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
			t.Fatal("Read failed:", err)
		} else if string(b) != sendData {
			t.Fatalf("Read %q; want %q", b, sendData)
		}
		if _, err := io.WriteString(remote, recvData); err != nil {
			t.Fatalf("Writing %q failed: %v", recvData, err)
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
	fwd, err := NewForwarder(nil, "127.0.0.1:0", "192.0.2.1:12345", func(err error) { ch <- err })
	if err != nil {
		t.Fatal("NewForwarder failed:", err)
	}
	defer fwd.Close()

	// Make the forwarder receive an error when it tries to open the remote connection,
	connErr := errors.New("intentional error")
	fwd.connFunc = func() (net.Conn, error) { return nil, connErr }

	// We should be able to establish a connection to the forwarder's local address,
	// but the error handler should receive the connection error.
	conn, err := net.Dial("tcp", fwd.LocalAddr().String())
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
