// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundletest

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	gotesting "testing"

	"google.golang.org/grpc"

	"chromiumos/tast/internal/bundle/fakebundle"
	"chromiumos/tast/internal/fakesshserver"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/sshtest"
)

// Env contains information needed to interact with the testing environment
// set up.
type Env struct {
	rootDir          string
	primaryServer    *fakesshserver.Server
	companionServers map[string]*fakesshserver.Server
}

var (
	localBundleDir  = "bundles/local"
	remoteBundleDir = "bundles/remote"

	keyFile = "id_rsa"
)

// SetUp sets up fake bundles with given parameters. Returned Env contains
// handles to interact with the environment that has been set up.
// connect instructs to make connections to remote bundles.
func SetUp(t *gotesting.T, opts ...Option) *Env {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}

	rootDir := t.TempDir()
	for _, dir := range []string{localBundleDir, remoteBundleDir} {
		if err := os.MkdirAll(filepath.Join(rootDir, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Set up local bundles.
	fullLocalBundleDir := filepath.Join(rootDir, localBundleDir)
	fakebundle.InstallAt(t, fullLocalBundleDir, cfg.localBundles...)

	userKey, hostKey := sshtest.MustGenerateKeys()
	keyData := pem.EncodeToMemory(&pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   x509.MarshalPKCS1PrivateKey(userKey),
	})
	if err := ioutil.WriteFile(filepath.Join(rootDir, keyFile), keyData, 0600); err != nil {
		t.Fatal(err)
	}

	primaryServer, err := startServer(userKey.PublicKey, fullLocalBundleDir, hostKey, cfg.primaryDUT)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(primaryServer.Stop)

	companionServers := make(map[string]*fakesshserver.Server)

	for role, dcfg := range cfg.companionDUTs {
		srv, err := startServer(userKey.PublicKey, fullLocalBundleDir, hostKey, dcfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(srv.Stop)

		companionServers[role] = srv
	}

	// Set up remote bundles.
	fakebundle.InstallAt(t, filepath.Join(rootDir, remoteBundleDir), cfg.remoteBundles...)
	return &Env{
		rootDir:          rootDir,
		primaryServer:    primaryServer,
		companionServers: companionServers,
	}
}

func startServer(user rsa.PublicKey, bundleDir string, host *rsa.PrivateKey, dcfg *DUTConfig) (*fakesshserver.Server, error) {
	hs := []fakesshserver.Handler{
		fakesshserver.ShellHandler(fmt.Sprintf("exec env %s", bundleDir)),
	}
	if dcfg != nil {
		hs = append(hs, dcfg.ExtraSSHHandlers...)
	}
	return fakesshserver.Start(&user, host, hs)
}

// KeyFile returns key file path.
func (e *Env) KeyFile() string {
	return filepath.Join(e.rootDir, keyFile)
}

// RemoteBundleDir returns absolute path of the remote bundle directory.
func (e *Env) RemoteBundleDir() string {
	return filepath.Join(e.rootDir, remoteBundleDir)
}

// LocalBundleDir returns absolute path of the local bundle directory.
func (e *Env) LocalBundleDir() string {
	return filepath.Join(e.rootDir, localBundleDir)
}

// PrimaryServer returns primary server address.
func (e *Env) PrimaryServer() string {
	return e.primaryServer.Addr().String()
}

// CompanionDUTs returns companion DUTs' role to address mapping.
func (e *Env) CompanionDUTs() map[string]string {
	res := make(map[string]string)
	for role, server := range e.companionServers {
		res[role] = server.Addr().String()
	}
	return res
}

// DialRemoteBundle makes a connection to the remote bundle with the given name.
func (e *Env) DialRemoteBundle(ctx context.Context, t *testing.T, name string) *grpc.ClientConn {
	cl, err := rpc.DialExec(
		ctx,
		filepath.Join(e.rootDir, remoteBundleDir, name),
		false,
		&protocol.HandshakeRequest{
			BundleInitParams: &protocol.BundleInitParams{
				BundleConfig: &protocol.BundleConfig{
					PrimaryTarget: &protocol.TargetDevice{
						DutConfig: &protocol.DUTConfig{
							SshConfig: &protocol.SSHConfig{
								ConnectionSpec: e.PrimaryServer(),
								KeyFile:        filepath.Join(e.rootDir, keyFile),
								KeyDir:         e.rootDir,
							},
						},
						BundleDir: e.LocalBundleDir(),
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	return cl.Conn()
}
