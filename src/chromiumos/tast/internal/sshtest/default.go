// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sshtest

import (
	"crypto/rsa"
	"errors"
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
	Srvs        []*SSHServer
	UserKeyFile string
}

// NewTestData initializes and returns a TestData struct. Panics on error.
func NewTestData(handlers ...ExecHandler) *TestData {
	if len(handlers) == 0 {
		panic(errors.New("no handler is specfied"))
	}
	userKey, hostKey := MustGenerateKeys()
	var servers []*SSHServer
	for _, handler := range handlers {
		srv, err := NewSSHServer(&userKey.PublicKey, hostKey, handler)
		if err != nil {
			for _, srv := range servers {
				srv.Close()
			}
			panic(err)
		}
		servers = append(servers, srv)
	}
	userKeyFile, err := WriteKey(userKey)
	if err != nil {
		for _, srv := range servers {
			srv.Close()
		}
		panic(err)
	}
	return &TestData{
		Srvs:        servers,
		UserKeyFile: userKeyFile,
	}
}

// Close stops the SSHServer and deletes the user key file.
func (d *TestData) Close() {
	for _, s := range d.Srvs {
		s.Close()
	}
	os.Remove(d.UserKeyFile)
}
