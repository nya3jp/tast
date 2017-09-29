// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package test provides support code for testing the host package.
package test // import "chromiumos/tast/common/host/test"

import (
	"bytes"
	"crypto/rsa"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os/exec"
	"syscall"

	"golang.org/x/crypto/ssh"
)

const (
	// sshMsgIgnore is the SSH global message sent to ping the host.
	// See RFC 4253 11.2, "Ignored Data Message".
	sshMsgIgnore = "SSH_MSG_IGNORE"

	// maxStringLen contains the maximum length for a string payload.
	maxStringLen = 2048
)

// SSHServer implements a somewhat-functional SSH server that listens on localhost
// and runs commands in response to "exec" requests. While the server requires
// authentication via a RSA keypair, it also refuses to run commands that haven't
// been registered via out-of-band requests.
//
// It is based on the ssh package's NewServerConn example.
type SSHServer struct {
	cfg         *ssh.ServerConfig
	listener    net.Listener
	nextCmd     string
	answerPings bool
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
func NewSSHServer(pk *rsa.PublicKey, hk *rsa.PrivateKey) (*SSHServer, error) {
	cfg, err := newServerConfig(pk, hk)
	if err != nil {
		return nil, err
	}
	ls, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &SSHServer{cfg, ls, "", true}

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

// NextCmd registers the next command that will be requested by the client.
func (s *SSHServer) NextCmd(cmd string) {
	s.nextCmd = cmd
}

// AnswerPings controls whether the server should reply to SSH_MSG_IGNORE ping
// requests or ignore them.
func (s *SSHServer) AnswerPings(v bool) {
	s.answerPings = v
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
		ch, chReqs, err := newChan.Accept()
		if err != nil {
			// TODO(derat): Do something with the error?
			continue
		}
		go s.handleChannel(ch, chReqs)
	}
	return nil
}

// handleChannel services a channel. Only "exec" requests are supported.
func (s *SSHServer) handleChannel(ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer ch.Close()
	for req := range reqs {
		if req.Type != "exec" {
			req.Reply(false, nil)
			return
		}
		cl, err := readStringPayload(req.Payload)
		if err != nil || cl != s.nextCmd {
			req.Reply(false, nil)
			return
		}
		s.nextCmd = ""

		req.Reply(true, nil)

		cmd := exec.Command("/bin/sh", "-c", cl)
		cmd.Stdout = ch
		cmd.Stderr = ch.Stderr()
		cmd.Stdin = ch
		status := 0
		if err = cmd.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				if ws, ok := ee.Sys().(syscall.WaitStatus); ok {
					status = ws.ExitStatus()
				}
			}
		}
		ch.SendRequest("exit-status", false, makeIntPayload(uint32(status)))
		return
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
