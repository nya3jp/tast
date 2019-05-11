// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package host

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
	connFunc func() (net.Conn, error) // opens remote conns; can be replaced by unit tests
	ls       net.Listener             // listens for local conns

	errFunc func(error) // called when error is encountered while forwarding
	mutex   sync.Mutex  // protects errFunc
}

// NewForwarder returns a Forwarder that listens at localAddr and instructs the SSH server to
// open a new connection to remoteAddr for each incoming connection. Traffic is forwarded
// between the local and remote connections via the SSH connection.
//
// localAddr and remoteAddr are passed to net.Listen and net.ResolveTCPAddr, respectively,
// and typically take the form "host:port" or "ip:port". Note that resolution of remoteAddr
// happens on the local system and not on the SSH server.
func NewForwarder(ssh *SSH, localAddr, remoteAddr string) (*Forwarder, error) {
	f := Forwarder{
		connFunc: func() (net.Conn, error) {
			addr, err := net.ResolveTCPAddr("tcp", remoteAddr)
			if err != nil {
				return nil, err
			}
			return ssh.DialTCP(addr)
		},
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
					f.errFunc(err)
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

// SetErrorFunc sets fn to be called in response to forwarding errors.
func (f *Forwarder) SetErrorFunc(fn func(error)) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.errFunc = fn
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
