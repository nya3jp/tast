// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import (
	"io"
	"net"
	"sync"
)

// Forwarder creates a local listener that forwards TCP connections to a remote host
// over an already-established SSH connection.
//
// A pictoral explanation:
//
//                 Local               |    SSH Host    |   Remote
//  -----------------------------------+----------------+-------------
//  [client] <- TCP -> [Forwarder] <- SSH -> [sshd] <- TCP -> [server]
type Forwarder struct {
	connFunc func() (net.Conn, error) // opens remote conns
	ls       net.Listener             // listens for local conns

	errFunc func(error) // called when error is encountered while forwarding; may be nil
	mutex   sync.Mutex  // protects errFunc
}

// newForwarder returns a Forwarder that listens at localAddr and calls connFunc to open a remote
// connection in response to each incoming connection. Traffic is copied between the local and
// remote connections.
//
// localAddr is passed to net.Listen and typically takes the form "host:port" or "ip:port".
// If non-nil, errFunc will be invoked asynchronously on a goroutine with connection or forwarding errors.
func newForwarder(localAddr string, connFunc func() (net.Conn, error), errFunc func(error)) (*Forwarder, error) {
	f := Forwarder{
		connFunc: connFunc,
		errFunc:  errFunc,
	}

	var err error
	if f.ls, err = net.Listen("tcp", localAddr); err != nil {
		return nil, err
	}

	// Start a goroutine that services the local listener and launches
	// a new goroutine to handle each incoming connection.
	go func() {
		for {
			local, err := f.ls.Accept()
			if err != nil {
				break
			}
			go func() {
				if err := f.handleConn(local); err != nil {
					f.mutex.Lock()
					if f.errFunc != nil {
						f.errFunc(err)
					}
					f.mutex.Unlock()
				}
			}()
		}
	}()

	return &f, nil
}

// Close stops listening for incoming local connections.
func (f *Forwarder) Close() error {
	f.mutex.Lock()
	f.errFunc = nil
	f.mutex.Unlock()
	return f.ls.Close()
}

// LocalAddr returns the local address used to listen for connections.
func (f *Forwarder) LocalAddr() net.Addr {
	return f.ls.Addr()
}

// handleConn establishes a new connection to the remote address using connFunc
// and copies data between it and local. It closes local before returning.
func (f *Forwarder) handleConn(local net.Conn) error {
	defer local.Close()

	remote, err := f.connFunc()
	if err != nil {
		return err
	}
	defer remote.Close()

	ch := make(chan error)
	go func() {
		_, err := io.Copy(local, remote)
		ch <- err
	}()
	go func() {
		_, err := io.Copy(remote, local)
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
