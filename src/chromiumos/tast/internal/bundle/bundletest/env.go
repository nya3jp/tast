// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundletest

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"os"
	"path/filepath"
	gotesting "testing"

	"google.golang.org/grpc"

	"chromiumos/tast/internal/bundle/fakebundle"
	"chromiumos/tast/internal/fakesshserver"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/sshtest"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/testutil"
)

// Env contains information needed to interact with the testing environment
// set up.
type Env struct {
	rootDir          string
	primaryServer    *fakesshserver.Server
	companionServers map[string]*fakesshserver.Server
	cfg              *config
}

var (
	localBundleDir  = "bundles/local"
	remoteBundleDir = "bundles/remote"
	localDataDir    = "localdata"
	remoteDataDir   = "remotedata"
	localOutDir     = "tmp/out/local"
	remoteOutDir    = "tmp/out/remote"

	keyFile = "id_rsa"
)

// SetUp sets up fake bundles with given parameters. Returned Env contains
// handles to interact with the environment that has been set up.
// connect instructs to make connections to remote bundles.
func SetUp(t *gotesting.T, opts ...Option) *Env {
	cfg := &config{}
	for _, o := range opts {
		o(cfg)
	}

	rootDir := t.TempDir()
	for _, dir := range []string{localBundleDir, remoteBundleDir} {
		if err := os.MkdirAll(filepath.Join(rootDir, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Set up local bundles.
	fakebundle.InstallAt(t, filepath.Join(rootDir, localBundleDir), cfg.localBundles...)

	userKey, hostKey := sshtest.MustGenerateKeys()
	keyData := pem.EncodeToMemory(&pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   x509.MarshalPKCS1PrivateKey(userKey),
	})
	if err := ioutil.WriteFile(filepath.Join(rootDir, keyFile), keyData, 0600); err != nil {
		t.Fatal(err)
	}

	primaryServer, err := startServer(userKey.PublicKey, rootDir, hostKey, cfg.primaryDUT)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(primaryServer.Stop)

	companionServers := make(map[string]*fakesshserver.Server)

	for role, dcfg := range cfg.companionDUTs {
		srv, err := startServer(userKey.PublicKey, rootDir, hostKey, dcfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(srv.Stop)

		companionServers[role] = srv
	}

	// Set up remote bundles.
	fakebundle.InstallAt(t, filepath.Join(rootDir, remoteBundleDir), cfg.remoteBundles...)

	// Set up data files.
	if err := testutil.WriteFiles(filepath.Join(rootDir, localDataDir), cfg.localData); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFiles(filepath.Join(rootDir, remoteDataDir), cfg.remoteData); err != nil {
		t.Fatal(err)
	}

	return &Env{
		rootDir:          rootDir,
		primaryServer:    primaryServer,
		companionServers: companionServers,
		cfg:              cfg,
	}
}

func startServer(user rsa.PublicKey, rootDir string, host *rsa.PrivateKey, dcfg *DUTConfig) (*fakesshserver.Server, error) {
	bundleDir := filepath.Join(rootDir, localBundleDir)
	outDir := filepath.Join(rootDir, localOutDir)
	hs := []fakesshserver.Handler{
		fakesshserver.ShellHandler("exec env " + bundleDir),
		// linuxssh.GetAndDeleteFile
		fakesshserver.ShellHandler("exec tar -c --gzip -C " + outDir),
		fakesshserver.ShellHandler("exec rm -rf -- " + outDir),
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

// RemoteDataDir returns absolute path of the remote bundle directory.
func (e *Env) RemoteDataDir() string {
	return filepath.Join(e.rootDir, remoteDataDir)
}

// LocalDataDir returns absolute path of the local bundle directory.
func (e *Env) LocalDataDir() string {
	return filepath.Join(e.rootDir, localDataDir)
}

// RemoteOutDir returns the absolute path to the remote bundle's temporary output
// directory.
func (e *Env) RemoteOutDir() string {
	return filepath.Join(e.rootDir, remoteOutDir)
}

// LocalOutDir returns the absolute path to the local bundle's temporary output
// directory.
func (e *Env) LocalOutDir() string {
	return filepath.Join(e.rootDir, localOutDir)
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
func (e *Env) DialRemoteBundle(ctx context.Context, t *gotesting.T, name string) *grpc.ClientConn {
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

// RunConfig returns a RunConfig struct with reasonable default values to be
// used in unit tests.
func (e *Env) RunConfig() *protocol.RunConfig {
	var tests []string
	for _, reg := range append(append(([]*testing.Registry)(nil), e.cfg.localBundles...), e.cfg.remoteBundles...) {
		for _, t := range reg.AllTests() {
			tests = append(tests, t.Name)
		}
	}
	return &protocol.RunConfig{
		Tests: tests,
		Features: &protocol.Features{
			CheckDeps: true,
		},
		Dirs: &protocol.RunDirectories{
			DataDir: e.RemoteDataDir(),
			OutDir:  e.RemoteOutDir(),
		},
		Target: &protocol.RunTargetConfig{
			Dirs: &protocol.RunDirectories{
				DataDir: e.LocalDataDir(),
				OutDir:  e.LocalOutDir(),
			},
		},
	}
}
