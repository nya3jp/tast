// Copyright 2023 The ChromiumOS Authors
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

func TestForwarderPropagatesEOF(t *testing.T) {
	dstSrv, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("Failed to listen for destination:", err)
	}
	defer dstSrv.Close()

	dstAccepted := make(chan net.Conn, 1)
	go func() {
		c, err := dstSrv.Accept()
		if err == nil {
			dstAccepted <- c
		}
	}()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("Failed to listen:", err)
	}
	fwd, err := newForwarder(listener, func() (net.Conn, error) {
		return net.Dial("tcp", dstSrv.Addr().String())
	}, nil)
	if err != nil {
		t.Fatal("newForwarder failed:", err)
	}
	defer fwd.Close()

	client, err := net.Dial("tcp", fwd.ListenAddr().String())
	if err != nil {
		t.Fatal("Dial failed:", err)
	}
	server := <-dstAccepted
	defer server.Close()

	// Closing the client must propagate EOF to the remote peer promptly so
	// the remote server exits and handleConn does not leak.
	client.Close()
	server.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := server.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("Remote read after client close = %v; want io.EOF", err)
	}
}

func TestForwarderCloseClosesActiveConns(t *testing.T) {
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("Failed to listen:", err)
	}
	fwd, err := newForwarder(listener, func() (net.Conn, error) {
		return local, nil
	}, func(err error) {})
	if err != nil {
		t.Fatal("newForwarder failed:", err)
	}

	client, err := net.Dial("tcp", fwd.ListenAddr().String())
	if err != nil {
		t.Fatal("Dial failed:", err)
	}
	defer client.Close()

	// Exchange a byte so handleConn has tracked both connections and entered io.Copy.
	if _, err := client.Write([]byte{'x'}); err != nil {
		t.Fatal("Write failed:", err)
	}
	remote.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(remote, make([]byte, 1)); err != nil {
		t.Fatal("Initial remote read failed:", err)
	}

	if err := fwd.Close(); err != nil {
		t.Fatal("Close failed:", err)
	}

	remote.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := remote.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Errorf("remote.Read after Forwarder.Close = %v; want io.EOF", err)
	}
	client.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := client.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Errorf("client.Read after Forwarder.Close = %v; want io.EOF", err)
	}
}
