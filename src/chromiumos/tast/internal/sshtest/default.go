// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sshtest

import (
	"crypto/rsa"
	"os"
)

const (
	defaultKeyBits = 1024
)

// MustGenerateKeys can be called from a test file's init function to generate
// 1024-bit user and host keys. Panics on error.
func MustGenerateKeys() (userKey, hostKey *rsa.PrivateKey) {
	var err error
	if userKey, hostKey, err = GenerateKeys(defaultKeyBits); err != nil {
		panic(err)
	}
	return userKey, hostKey
}

// TestData contains common data that can be used by tests that interact with an SSHServer.
type TestData struct { // NOLINT
	Srv           *SSHServer
	UserKeyFile   string
	additionalSrv []*SSHServer
	userKey       *rsa.PrivateKey
	hostKey       *rsa.PrivateKey
}

// NewTestData initializes and returns a TestData struct. Panics on error.
func NewTestData(handler ExecHandler) *TestData {
	userKey, hostKey = MustGenerateKeys()
	var err error
	srv, err := NewSSHServer(&userKey.PublicKey, hostKey, handler)
	if err != nil {
		panic(err)
	}
	userKeyFile, err := WriteKey(userKey)
	if err != nil {
		srv.Close()
		panic(err)
	}
	return &TestData{
		Srv:         srv,
		UserKeyFile: userKeyFile,
		userKey:     userKey,
		hostKey:     hostKey,
	}
}

// NewAdditionalSSHServer create additional ssh server in TestData struct for testing.
func (d *TestData) NewAdditionalSSHServer(handler ExecHandler) *SSHServer {
	srv, err := NewSSHServer(&d.userKey.PublicKey, d.hostKey, handler)
	if err != nil {
		panic(err)
	}
	d.additionalSrv = append(d.additionalSrv, srv)
	return srv
}

// Close stops the SSHServer and deletes the user key file.
func (d *TestData) Close() {
	d.Srv.Close()
	for _, s := range d.additionalSrv {
		s.Close()
	}
	os.Remove(d.UserKeyFile)
}
