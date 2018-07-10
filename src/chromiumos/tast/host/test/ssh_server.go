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
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"sync"
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
// It is based on the ssh package's NewServerConn example and can be used concurrently
// from multiple goroutines.
type SSHServer struct {
	cfg      *ssh.ServerConfig
	listener net.Listener

	mutex       sync.Mutex           // protects following fields
	nextCmd     string               // next expected "exec" command to actually run
	fakeCmds    map[string]cmdResult // "exec" command lines to canned results to return
	answerPings bool                 // if true, ping requests will be answered
}

// cmdResult holds the result that should be returned in response to a command.
type cmdResult struct {
	exitStatus     int
	stdout, stderr []byte
	stdinDest      io.Writer
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
	s := &SSHServer{
		cfg:         cfg,
		listener:    ls,
		fakeCmds:    make(map[string]cmdResult),
		answerPings: true,
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

// NextCmd sets the next command expected to be sent in an "exec" request.
// The supplied command will actually be executed.
func (s *SSHServer) NextCmd(cmd string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.nextCmd = cmd
}

// FakeCmd configures the result to be sent for an "exec" request exactly matching cmd.
func (s *SSHServer) FakeCmd(cmd string, exitStatus int, stdout, stderr []byte, stdinDest io.Writer) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.fakeCmds[cmd] = cmdResult{exitStatus, stdout, stderr, stdinDest}
}

// AnswerPings controls whether the server should reply to SSH_MSG_IGNORE ping requests or ignore them.
func (s *SSHServer) AnswerPings(v bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
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
			if err := s.handleExec(ch, req); err != nil {
				log.Print("SSH exec command failed: ", err)
			} else {
				// Only one "exec" request can succeed per channel (see RFC 4254 6.5).
				return
			}
		default:
			req.Reply(false, nil)
		}
	}
}

// handleExec handles "exec" request req received on ch.
// It writes a reply and any additional required data (e.g. exit status).
func (s *SSHServer) handleExec(ch ssh.Channel, req *ssh.Request) error {
	cl, err := readStringPayload(req.Payload)
	if err != nil {
		req.Reply(false, nil)
		return err
	}
	if cl == "" {
		req.Reply(false, nil)
		return errors.New("empty command")
	}

	s.mutex.Lock()
	fakeCmd, haveFakeCmd := s.fakeCmds[cl]
	nextCmd := s.nextCmd
	s.nextCmd = ""
	s.mutex.Unlock()

	status := 0
	if haveFakeCmd {
		req.Reply(true, nil)
		if fakeCmd.stdinDest != nil {
			io.Copy(fakeCmd.stdinDest, ch)
		}
		ch.Write(fakeCmd.stdout)
		ch.Stderr().Write(fakeCmd.stderr)
		status = fakeCmd.exitStatus
	} else if cl == nextCmd {
		req.Reply(true, nil)
		cmd := exec.Command("/bin/sh", "-c", cl)
		cmd.Stdout = ch
		cmd.Stderr = ch.Stderr()
		cmd.Stdin = ch
		if err = cmd.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				if ws, ok := ee.Sys().(syscall.WaitStatus); ok {
					status = ws.ExitStatus()
				}
			}
		}
	} else {
		req.Reply(false, nil)
		return fmt.Errorf("unexpected command %q", cl)
	}

	ch.SendRequest("exit-status", false, makeIntPayload(uint32(status)))
	return nil
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
