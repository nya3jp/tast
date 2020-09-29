// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import (
	"io"
	"net"
	"sync"
)

// Forwarder creates a listener that forwards TCP connections to another host
// over an already-established SSH connection.
//
// A pictoral explanation:
//
//                 Local               |    SSH Host    |   Remote
//  -----------------------------------+----------------+-------------
// (local-to-remote)
//  [client] <- TCP -> [Forwarder] <- SSH -> [sshd] <- TCP -> [server]
// (remote-to-local)
//  [server] <- TCP -> [Forwarder] <- SSH -> [sshd] <- TCP -> [client]
type Forwarder struct {
	connFunc func() (net.Conn, error) // opens remote conns
	ls       net.Listener             // listens for the forwarded conns

	errFunc func(error) // called when error is encountered while forwarding; may be nil
	mutex   sync.Mutex  // protects errFunc
}

// newForwarder returns a Forwarder that calls connFunc to open a connection in response to
// each incoming connection. Traffic is copied between the source and destination connections.
//
// If non-nil, errFunc will be invoked asynchronously on a goroutine with connection or forwarding errors.
func newForwarder(listener net.Listener, connFunc func() (net.Conn, error), errFunc func(error)) (*Forwarder, error) {
	f := Forwarder{
		connFunc: connFunc,
		errFunc:  errFunc,
		ls:       listener,
	}

	// Start a goroutine that services the listener and launches
	// a new goroutine to handle each incoming connection.
	go func() {
		for {
			local, err := f.ls.Accept()
			if err != nil {
				break
			}
			go func() {
				if err := f.handleConn(local); err != nil && f.errFunc != nil {
					f.mutex.Lock()
					f.errFunc(err)
					f.mutex.Unlock()
				}
			}()
		}
	}()

	return &f, nil
}

// Close stops listening for incoming connections.
func (f *Forwarder) Close() error {
	f.mutex.Lock()
	f.errFunc = nil
	f.mutex.Unlock()
	return f.ls.Close()
}

// LocalAddr returns the address used to listen for connections.
// Deprecated. Use ListenAddr instead.
func (f *Forwarder) LocalAddr() net.Addr {
	return f.ListenAddr()
}

// ListenAddr returns the address used to listen for connections.
func (f *Forwarder) ListenAddr() net.Addr {
	return f.ls.Addr()
}

// handleConn establishes a new connection to the destination port using connFunc
// and copies data between it and src. It closes src before returning.
func (f *Forwarder) handleConn(src net.Conn) error {
	defer src.Close()

	dst, err := f.connFunc()
	if err != nil {
		return err
	}
	defer dst.Close()

	ch := make(chan error)
	go func() {
		_, err := io.Copy(src, dst)
		ch <- err
	}()
	go func() {
		_, err := io.Copy(dst, src)
		ch <- err
	}()

	var firstErr error
	for i := 0; i < 2; i++ {
		if err := <-ch; err != io.EOF && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
