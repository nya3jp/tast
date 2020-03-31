// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sshtest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
)

// GenerateKeys generates SSH user and host keys of size bits.
// This can be time-consuming, so a test file may want to only call this once in
// its init function and reuse the results.
func GenerateKeys(bits int) (userKey, hostKey *rsa.PrivateKey, err error) {
	if userKey, err = rsa.GenerateKey(rand.Reader, bits); err != nil {
		return nil, nil, fmt.Errorf("failed to generate user RSA key: %v", err)
	}
	if hostKey, err = rsa.GenerateKey(rand.Reader, bits); err != nil {
		return nil, nil, fmt.Errorf("failed to generate host RSA key: %v", err)
	}
	return userKey, hostKey, nil
}

// WriteKey writes key to a temporary file and returns its path.
// The caller is responsible for unlinking the temp file when complete.
func WriteKey(key *rsa.PrivateKey) (path string, err error) {
	data := pem.EncodeToMemory(&pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   x509.MarshalPKCS1PrivateKey(key),
	})

	f, err := ioutil.TempFile("", "tast_unittest_ssh_key.")
	if err != nil {
		return "", err
	}
	defer f.Close()

	if err = f.Chmod(0600); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	if _, err = f.Write(data); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
