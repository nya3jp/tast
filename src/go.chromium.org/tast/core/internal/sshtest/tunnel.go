// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sshtest

import (
	"io"
	"net"
	"sync/atomic"
)

// Tunnel forwards TCP connections to an address. It stands in for a tunnel
// forwarded over an SSH connection.
type Tunnel struct {
	listener net.Listener
	conns    atomic.Int32
}

// NewTunnel starts a Tunnel to addr. Caller must call Close after use.
func NewTunnel(addr string) (*Tunnel, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	t := &Tunnel{listener: l}
	go func() {
		for {
			src, err := l.Accept()
			if err != nil {
				return
			}
			t.conns.Add(1)
			go func() {
				defer src.Close()
				dst, err := net.Dial("tcp", addr)
				if err != nil {
					return
				}
				defer dst.Close()
				go func() {
					defer dst.Close()
					io.Copy(dst, src)
				}()
				io.Copy(src, dst)
			}()
		}
	}()
	return t, nil
}

// Addr returns the address the tunnel listens on.
func (t *Tunnel) Addr() net.Addr { return t.listener.Addr() }

// Conns returns the number of connections the tunnel has accepted.
func (t *Tunnel) Conns() int { return int(t.conns.Load()) }

// Close stops accepting connections.
func (t *Tunnel) Close() error { return t.listener.Close() }
