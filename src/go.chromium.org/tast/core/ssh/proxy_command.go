// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import (
	"bytes"
	"context"
	"io"
	"net"
	"os/exec"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/tastuseonly/logging"
)

// DialProxyCommand creates a new connection using the specified proxy command.
func DialProxyCommand(ctx context.Context, hostPort, proxyCommand string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return nil, err
	}
	// NOTE: `man ssh_config(5)` lists more tokens, but currently only %h and %p are supported.
	replacer := strings.NewReplacer("%%", "%", "%h", host, "%p", port)
	proxyCommandReplaced := replacer.Replace(proxyCommand)

	logging.Debugf(ctx, "Connecting with proxy command: %s", proxyCommandReplaced)
	args := strings.Split(proxyCommandReplaced, " ")
	cmd := exec.Command(args[0], args[1:]...)

	// Forward read and write requests to stdin and stdout of the proxy command.
	var conn proxyCommandConn
	if conn.WriteCloser, err = cmd.StdinPipe(); err != nil {
		return nil, err
	}
	if conn.ReadCloser, err = cmd.StdoutPipe(); err != nil {
		return nil, err
	}
	cmd.Stderr = &conn.stderrBuffer

	if err = cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		if err = cmd.Wait(); err != nil {
			logging.Infof(ctx, "Proxy command failed: comand = '%s', err = '%s', stderr = '%s'", proxyCommandReplaced, err, conn.stderrBuffer.String())
		}
	}()
	return &conn, nil
}

// proxyCommandConn implements net.Conn and net.Addr.
type proxyCommandConn struct {
	io.ReadCloser
	io.WriteCloser
	stderrBuffer bytes.Buffer
}

func (c *proxyCommandConn) Close() error {
	readErr := c.ReadCloser.Close()
	writeErr := c.WriteCloser.Close()
	if readErr == nil {
		return writeErr
	}
	return readErr
}

func (c *proxyCommandConn) LocalAddr() net.Addr {
	return c
}

func (c *proxyCommandConn) RemoteAddr() net.Addr {
	return c
}

func (*proxyCommandConn) SetDeadline(t time.Time) error {
	return errors.New("not supported")
}

func (*proxyCommandConn) SetReadDeadline(t time.Time) error {
	return errors.New("not supported")
}

func (*proxyCommandConn) SetWriteDeadline(t time.Time) error {
	return errors.New("not supported")
}

func (proxyCommandConn) Network() string {
	return "proxycommand" // Placeholder network name.
}

func (proxyCommandConn) String() string {
	return "0.0.0.0:0" // Placeholder network address string.
}
