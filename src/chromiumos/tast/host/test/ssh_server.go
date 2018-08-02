// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package test provides support code for testing the host package.
package test

import (
	"bytes"
	"crypto/rsa"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	// sshMsgIgnore is the SSH global message sent to ping the host.
	// See RFC 4253 11.2, "Ignored Data Message".
	sshMsgIgnore = "SSH_MSG_IGNORE"

	// maxStringLen contains the maximum length for a string payload.
	maxStringLen = 2048
)

// SSHServer implements an SSH server based on the ssh package's NewServerConn
// example that listens on localhost and performs authentication via an RSA keypair.
//
// Only "exec" requests and pings (using SSH_MSG_IGNORE) are supported.
// "exec" requests are handled using a caller-supplied function.
type SSHServer struct {
	cfg      *ssh.ServerConfig
	listener net.Listener

	answerPings  bool          // if true, ping requests will be answered
	sessionDelay time.Duration // delay before starting new sessions
	execHandler  ExecHandler   // called to handle "exec" requests
}

// newServerConfig returns a new configuration for a server using host key hk
// and accepting public key authentication using pk.
func newServerConfig(pk *rsa.PublicKey, hk *rsa.PrivateKey) (*ssh.ServerConfig, error) {
	pub, err := ssh.NewPublicKey(pk)
	if err != nil {
		return nil, fmt.Errorf("failed to generate SSH public key: %v", err)
	}
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(c ssh.ConnMetadata, pubKey ssh.PublicKey) (*ssh.Permissions, error) {
			if subtle.ConstantTimeCompare(pubKey.Marshal(), pub.Marshal()) == 1 {
				return &ssh.Permissions{}, nil
			}
			return nil, fmt.Errorf("unknown public key for %q", c.User())
		},
	}

	signer, err := ssh.NewSignerFromKey(hk)
	if err != nil {
		return nil, fmt.Errorf("failed to generate host signer: %v", err)
	}
	cfg.AddHostKey(signer)

	return cfg, nil
}

// NewSSHServer creates an SSH server using host key hk and accepting public key authentication using pk.
// A random port bound to the local IPv4 interface is used.
func NewSSHServer(pk *rsa.PublicKey, hk *rsa.PrivateKey, handler ExecHandler) (*SSHServer, error) {
	cfg, err := newServerConfig(pk, hk)
	if err != nil {
		return nil, err
	}
	ls, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &SSHServer{
		cfg:         cfg,
		listener:    ls,
		answerPings: true,
		execHandler: handler,
	}

	go func() {
		for {
			conn, err := ls.Accept()
			if err != nil {
				log.Print("Shutting down")
				return
			}
			if err := s.handleConn(conn); err != nil {
				log.Print("Got error while handling connection: ", err)
			}
		}
	}()

	return s, nil
}

// Close instructs the server to stop listening for connections.
func (s *SSHServer) Close() error {
	return s.listener.Close()
}

// AnswerPings controls whether the server should reply to SSH_MSG_IGNORE ping requests or ignore them.
func (s *SSHServer) AnswerPings(v bool) {
	s.answerPings = v
}

// SessionDelay configures a delay used by the server before starting a new session.
func (s *SSHServer) SessionDelay(d time.Duration) {
	s.sessionDelay = d
}

// Addr returns the address on which the server is listening.
func (s *SSHServer) Addr() net.Addr {
	if s.listener == nil {
		panic("Server not listening")
	}
	return s.listener.Addr()
}

// handleConn services a new incoming connection on conn.
func (s *SSHServer) handleConn(conn net.Conn) error {
	_, chans, reqs, err := ssh.NewServerConn(conn, s.cfg)
	if err != nil {
		return fmt.Errorf("failed to handshake: %v", err)
	}

	go func() {
		for req := range reqs {
			if req.WantReply && (req.Type != sshMsgIgnore || s.answerPings) {
				req.Reply(false, nil)
			}
		}
	}()

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			newChan.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		time.Sleep(s.sessionDelay)

		ch, chReqs, err := newChan.Accept()
		if err != nil {
			return fmt.Errorf("failed to accept channel: %v", err)
		}
		go s.handleChannel(ch, chReqs)
	}
	return nil
}

// handleChannel services a channel. Only "exec" requests are supported.
func (s *SSHServer) handleChannel(ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer ch.Close()

	for req := range reqs {
		switch req.Type {
		case "exec":
			if cmd, err := readStringPayload(req.Payload); err != nil {
				log.Print("Failed to read command: ", err)
				req.Reply(false, nil)
			} else if s.execHandler == nil {
				log.Print("No exec handler configured")
				req.Reply(false, nil)
			} else {
				er := ExecReq{cmd, ch, req, false}
				s.execHandler(&er)
				if er.success {
					// Only one "exec" request can succeed per channel (see RFC 4254 6.5).
					return
				}
			}
		default:
			log.Printf("Unhandled request of type %q", req.Type)
			req.Reply(false, nil)
		}
	}
}

// readStringPayload reads and returns a length-prefixed string
// from a ssh.Request payload.
func readStringPayload(payload []byte) (string, error) {
	var slen uint32
	br := bytes.NewReader(payload)
	if err := binary.Read(br, binary.BigEndian, &slen); err != nil {
		return "", fmt.Errorf("failed to read length: %v", err)
	}
	if slen > maxStringLen {
		return "", fmt.Errorf("string length %v too big", slen)
	}

	b := make([]byte, slen)
	if err := binary.Read(br, binary.BigEndian, &b); err != nil {
		return "", fmt.Errorf("failed to read %v-byte string: %v", slen, err)
	}
	return string(b), nil
}

// makeIntPayload returns a SSH request payload containing v.
func makeIntPayload(v uint32) []byte {
	b := bytes.Buffer{}
	if err := binary.Write(&b, binary.BigEndian, &v); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// ExecReq is used to service an "exec" request.
// See RFC 4254 6.5, "Starting a Shell or a Command".
type ExecReq struct {
	// Cmd contains the command line to be executed.
	Cmd string

	ch  ssh.Channel
	req *ssh.Request

	success bool // reply passed to Start
}

// Start sends a reply to the request reporting the start of the command.
// If success is false, no further methods should be called.
// Otherwise, End should be called after the command finishes.
func (e *ExecReq) Start(success bool) error {
	e.success = success
	return e.req.Reply(success, nil)
}

// Read reads up to len(data) bytes of input supplied by the SSH client.
// The data should be passed to the executed command's stdin.
func (e *ExecReq) Read(data []byte) (int, error) { return e.ch.Read(data) }

// Write writes stdout produced by the executed command.
// It cannot be called after CloseOutput.
func (e *ExecReq) Write(data []byte) (int, error) { return e.ch.Write(data) }

// Stderr returns a ReadWriter used to write stderr produced by the executed command.
// It cannot be called after CloseOutput.
func (e *ExecReq) Stderr() io.ReadWriter { return e.ch.Stderr() }

// CloseOutput closes stdout and stderr.
func (e *ExecReq) CloseOutput() error { return e.ch.CloseWrite() }

// End reports the command's status code after execution finishes.
func (e *ExecReq) End(status int) error {
	_, err := e.ch.SendRequest("exit-status", false, makeIntPayload(uint32(status)))
	return err
}

// RunRealCmd runs e.Cmd synchronously, passing stdout, stderr, and stdin appropriately.
// It calls CloseOutput on completion and returns the process's status code.
// Callers should call Start(true) before RunRealCmd and End (with the returned status code) after.
// Callers must validate commands via an out-of-band mechanism before calling this; see host.SSH.AnnounceCmd.
func (e *ExecReq) RunRealCmd() int {
	defer e.CloseOutput()

	cmd := exec.Command("/bin/sh", "-c", e.Cmd)
	cmd.Stdout = e.ch
	cmd.Stderr = e.ch.Stderr()
	cmd.Stdin = e.ch
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			if ws, ok := ee.Sys().(syscall.WaitStatus); ok {
				return ws.ExitStatus()
			}
		}
		// Some problem probably occurred before the command could be started.
		return 1
	}
	return 0
}

// ExecHandler is a function that will be called repeatedly to handle "exec" requests.
// It will be called concurrently on multiple goroutines if multiple overlapping requests are received.
type ExecHandler func(req *ExecReq)
