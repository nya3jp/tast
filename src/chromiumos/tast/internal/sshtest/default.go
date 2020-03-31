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
	Srv         *SSHServer
	UserKeyFile string
}

// NewTestData initializes and returns a TestData struct. Panics on error.
func NewTestData(userKey, hostKey *rsa.PrivateKey, handler ExecHandler) *TestData {
	d := TestData{}
	var err error
	if d.Srv, err = NewSSHServer(&userKey.PublicKey, hostKey, handler); err != nil {
		panic(err)
	}
	if d.UserKeyFile, err = WriteKey(userKey); err != nil {
		d.Srv.Close()
		panic(err)
	}
	return &d
}

// Close stops the SSHServer and deletes the user key file.
func (d *TestData) Close() {
	d.Srv.Close()
	os.Remove(d.UserKeyFile)
}
