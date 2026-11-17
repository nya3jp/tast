// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	cryptossh "golang.org/x/crypto/ssh"

	"go.chromium.org/tast/core/internal/sshtest"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testutil"
)

var userKey, hostKey = sshtest.MustGenerateKeys()

func TestRetry(t *testing.T) {
	t.Parallel()
	srv, err := sshtest.NewSSHServer(&userKey.PublicKey, hostKey, func(*sshtest.ExecReq) {})
	if err != nil {
		t.Fatal("Failed starting server: ", err)
	}
	defer srv.Close()

	// Configure the server to reject the next two connections and let the client only retry once.
	srv.RejectConns(2)
	ctx := context.Background()
	if hst, err := sshtest.ConnectToServer(ctx, srv, userKey, &ssh.Options{ConnectRetries: 1}); err == nil {
		t.Error("Unexpectedly able to connect to server with inadequate retries")
		hst.Close(ctx)
	}

	// With two retries (i.e. three attempts), the connection should be successfully established.
	srv.RejectConns(2)
	if hst, err := sshtest.ConnectToServer(ctx, srv, userKey, &ssh.Options{ConnectRetries: 2}); err != nil {
		t.Error("Failed connecting to server despite adequate retries: ", err)
	} else {
		hst.Close(ctx)
	}
}

func TestPing(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	td.Srv.AnswerPings(true)
	if err := td.Hst.Ping(td.Ctx, time.Minute); err != nil {
		t.Errorf("Got error when pinging host: %v", err)
	}

	td.Srv.AnswerPings(false)
	if err := td.Hst.Ping(td.Ctx, time.Millisecond); err == nil {
		t.Errorf("Didn't get expected error when pinging host with short timeout")
	}

	// Cancel the context to simulate it having expired.
	td.Cancel()
	if err := td.Hst.Ping(td.Ctx, time.Minute); err == nil {
		t.Errorf("Didn't get expected error when pinging host with expired context")
	}
}

func TestKeyDir(t *testing.T) {
	t.Parallel()
	srv, err := sshtest.NewSSHServer(&userKey.PublicKey, hostKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	keyFile, err := sshtest.WriteKey(userKey)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(keyFile)

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	if err = os.Symlink(keyFile, filepath.Join(td, "testing_rsa")); err != nil {
		t.Fatal(err)
	}

	opt := ssh.Options{KeyDir: td}
	if err = ssh.ParseTarget(srv.Addr().String(), &opt); err != nil {
		t.Fatal(err)
	}
	hst, err := ssh.New(context.Background(), &opt)
	if err != nil {
		t.Fatal(err)
	}
	hst.Close(context.Background())
}

func TestGenerateRemoteAddress(t *testing.T) {
	t.Parallel()
	srv, err := sshtest.NewSSHServer(&userKey.PublicKey, hostKey, func(*sshtest.ExecReq) {})
	if err != nil {
		t.Fatal("Failed starting server: ", err)
	}
	defer srv.Close()

	ctx := context.Background()
	hst, err := sshtest.ConnectToServer(ctx, srv, userKey, &ssh.Options{})
	if err != nil {
		t.Fatal("Unexpectedly unable to connect to server: ", err)
	}
	defer hst.Close(ctx)

	got, _ := hst.GenerateRemoteAddress(2345)
	want := "127.0.0.1:2345"
	if got != want {
		t.Fatalf("hst.GenerateRemoteAddress(2345) = %q, want: %q", got, want)
	}
}

func TestProxyCommand(t *testing.T) {
	t.Parallel()
	srv, err := sshtest.NewSSHServer(&userKey.PublicKey, hostKey, nil)
	if err != nil {
		t.Fatal("Failed starting server: ", err)
	}
	defer srv.Close()

	ctx := context.Background()
	opt := ssh.Options{ProxyCommand: "socat tcp-connect:%h:%p STDIO"}
	hst, err := sshtest.ConnectToServer(ctx, srv, userKey, &opt)
	if err != nil {
		t.Fatal("Unexpectedly unable to connect to server: ", err)
	}
	defer hst.Close(ctx)
}

func TestProxyCommandNone(t *testing.T) {
	t.Parallel()
	srv, err := sshtest.NewSSHServer(&userKey.PublicKey, hostKey, nil)
	if err != nil {
		t.Fatal("Failed starting server: ", err)
	}
	defer srv.Close()

	ctx := context.Background()
	opt := ssh.Options{ProxyCommand: "none"}
	hst, err := sshtest.ConnectToServer(ctx, srv, userKey, &opt)
	if err != nil {
		t.Fatal("Unexpectedly unable to connect to server: ", err)
	}
	defer hst.Close(ctx)
}

func TestTunnel(t *testing.T) {
	t.Parallel()
	srv, err := sshtest.NewSSHServer(&userKey.PublicKey, hostKey, nil)
	if err != nil {
		t.Fatal("Failed starting server: ", err)
	}
	defer srv.Close()
	tunnel, err := sshtest.NewTunnel(srv.Addr().String())
	if err != nil {
		t.Fatal("Failed starting tunnel: ", err)
	}
	defer tunnel.Close()

	ctx := t.Context()
	opt := ssh.Options{
		TunnelAddress:  tunnel.Addr().String(),
		ProxyCommand:   "false",
		ConnectTimeout: time.Minute,
	}
	hst, err := sshtest.ConnectToServer(ctx, srv, userKey, &opt)
	if err != nil {
		t.Fatal("Unexpectedly unable to connect to server: ", err)
	}
	defer hst.Close(ctx)
	if n := tunnel.Conns(); n != 1 {
		t.Errorf("Tunnel got %d connections; want 1", n)
	}
}

func TestTunnelTimeout(t *testing.T) {
	t.Parallel()
	srv, err := sshtest.NewSSHServer(&userKey.PublicKey, hostKey, nil)
	if err != nil {
		t.Fatal("Failed starting server: ", err)
	}
	defer srv.Close()
	// This listener never accepts connections, like a tunnel over a dead
	// connection.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("Failed to listen: ", err)
	}
	defer l.Close()

	ctx := t.Context()
	opt := ssh.Options{TunnelAddress: l.Addr().String(), ConnectTimeout: 100 * time.Millisecond}
	hst, err := sshtest.ConnectToServer(ctx, srv, userKey, &opt)
	if err == nil {
		hst.Close(ctx)
		t.Fatal("Unexpectedly connected through a stalled tunnel")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ConnectToServer error = %v; want %v", err, context.DeadlineExceeded)
	}
}

func TestTunnelCanceled(t *testing.T) {
	t.Parallel()
	srv, err := sshtest.NewSSHServer(&userKey.PublicKey, hostKey, nil)
	if err != nil {
		t.Fatal("Failed starting server: ", err)
	}
	defer srv.Close()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("Failed to listen: ", err)
	}
	defer l.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	// ConnectTimeout is zero so only ctx cancellation can abort the stalled handshake.
	opt := ssh.Options{TunnelAddress: l.Addr().String()}
	hst, err := sshtest.ConnectToServer(ctx, srv, userKey, &opt)
	if err == nil {
		hst.Close(t.Context())
		t.Fatal("Unexpectedly connected through a stalled tunnel")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ConnectToServer error = %v; want %v", err, context.DeadlineExceeded)
	}
}

// recordingSigner wraps an ssh.AlgorithmSigner to record the signature format
// used during SSH key exchange. Implementing ssh.AlgorithmSigner ensures
// x/crypto/ssh advertises rsa-sha2-256/512 for RSA keys, while routing Sign
// through SignWithAlgorithm captures exact key-format matches (such as
// ssh-ed25519) where x/crypto/ssh calls Sign directly.
type recordingSigner struct {
	signer cryptossh.AlgorithmSigner
	mu     sync.Mutex
	algos  []string
}

var _ cryptossh.AlgorithmSigner = (*recordingSigner)(nil)

func newRecordingSigner(t *testing.T, key any) *recordingSigner {
	t.Helper()
	signer, err := cryptossh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal("Failed creating host signer: ", err)
	}
	algSigner, ok := signer.(cryptossh.AlgorithmSigner)
	if !ok {
		t.Fatalf("Signer %T does not implement ssh.AlgorithmSigner", signer)
	}
	return &recordingSigner{signer: algSigner}
}

func (s *recordingSigner) PublicKey() cryptossh.PublicKey {
	return s.signer.PublicKey()
}

func (s *recordingSigner) Sign(rand io.Reader, data []byte) (*cryptossh.Signature, error) {
	return s.SignWithAlgorithm(rand, data, "")
}

func (s *recordingSigner) SignWithAlgorithm(rand io.Reader, data []byte, algorithm string) (*cryptossh.Signature, error) {
	sig, err := s.signer.SignWithAlgorithm(rand, data, algorithm)
	if err == nil {
		s.mu.Lock()
		s.algos = append(s.algos, sig.Format)
		s.mu.Unlock()
	}
	return sig, err
}

func (s *recordingSigner) signedAlgos() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.algos)
}

func newHostCertSigner(t *testing.T, signer *recordingSigner, ca cryptossh.Signer) cryptossh.Signer {
	t.Helper()
	cert := &cryptossh.Certificate{
		Key:         signer.PublicKey(),
		CertType:    cryptossh.HostCert,
		ValidBefore: cryptossh.CertTimeInfinity,
	}
	if err := cert.SignCert(rand.Reader, ca); err != nil {
		t.Fatal("Failed signing host certificate: ", err)
	}
	certSigner, err := cryptossh.NewCertSigner(cert, signer)
	if err != nil {
		t.Fatal("Failed creating host certificate signer: ", err)
	}
	return certSigner
}

func connectWithHostKeys(t *testing.T, hostKeys ...cryptossh.Signer) {
	t.Helper()

	// Host key negotiation occurs during key exchange before user authentication.
	serverCfg := &cryptossh.ServerConfig{NoClientAuth: true}
	for _, hk := range hostKeys {
		serverCfg.AddHostKey(hk)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("Failed listening: ", err)
	}
	defer ln.Close()

	srvErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			srvErr <- err
			return
		}
		defer conn.Close()

		sConn, chans, reqs, err := cryptossh.NewServerConn(conn, serverCfg)
		if err != nil {
			srvErr <- err
			return
		}
		defer sConn.Close()

		go cryptossh.DiscardRequests(reqs)
		for ch := range chans {
			ch.Reject(cryptossh.UnknownChannelType, "unsupported")
		}
		srvErr <- nil
	}()

	ctx := t.Context()
	opt := ssh.Options{Hostname: ln.Addr().String()}
	hst, err := ssh.New(ctx, &opt)
	if err != nil {
		t.Fatal("ssh.New failed: ", err)
	}
	if err := hst.Close(ctx); err != nil {
		t.Error("hst.Close failed: ", err)
	}
	if err := <-srvErr; err != nil {
		t.Fatal("Server failed: ", err)
	}
}

func TestHostKeyAlgorithms(t *testing.T) {
	t.Parallel()

	ecdsaPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal("Failed generating ecdsa host key: ", err)
	}
	_, edPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal("Failed generating ed25519 host key: ", err)
	}
	caSigner, err := cryptossh.NewSignerFromKey(edPriv)
	if err != nil {
		t.Fatal("Failed creating CA signer: ", err)
	}

	t.Run("ed25519_preferred_over_rsa", func(t *testing.T) {
		rsaSigner := newRecordingSigner(t, hostKey)
		edSigner := newRecordingSigner(t, edPriv)

		// ChromeOS sshd offers only RSA and ED25519 host keys.
		connectWithHostKeys(t, rsaSigner, edSigner)

		if got := rsaSigner.signedAlgos(); len(got) != 0 {
			t.Errorf("RSA signed algos = %v; want []", got)
		}
		if got, want := edSigner.signedAlgos(), []string{cryptossh.KeyAlgoED25519}; !slices.Equal(got, want) {
			t.Errorf("ED25519 signed algos = %v; want %v", got, want)
		}
	})

	t.Run("ed25519_preferred", func(t *testing.T) {
		rsaSigner := newRecordingSigner(t, hostKey)
		ecdsaSigner := newRecordingSigner(t, ecdsaPriv)
		edSigner := newRecordingSigner(t, edPriv)

		// Offer RSA and ECDSA before ED25519 on the server so client preference decides.
		connectWithHostKeys(t, rsaSigner, ecdsaSigner, edSigner)

		if got := rsaSigner.signedAlgos(); len(got) != 0 {
			t.Errorf("RSA signed algos = %v; want []", got)
		}
		if got := ecdsaSigner.signedAlgos(); len(got) != 0 {
			t.Errorf("ECDSA signed algos = %v; want []", got)
		}
		if got, want := edSigner.signedAlgos(), []string{cryptossh.KeyAlgoED25519}; !slices.Equal(got, want) {
			t.Errorf("ED25519 signed algos = %v; want %v", got, want)
		}
	})

	t.Run("ecdsa_fallback", func(t *testing.T) {
		rsaSigner := newRecordingSigner(t, hostKey)
		ecdsaSigner := newRecordingSigner(t, ecdsaPriv)

		connectWithHostKeys(t, rsaSigner, ecdsaSigner)

		if got := rsaSigner.signedAlgos(); len(got) != 0 {
			t.Errorf("RSA signed algos = %v; want []", got)
		}
		if got, want := ecdsaSigner.signedAlgos(), []string{cryptossh.KeyAlgoECDSA256}; !slices.Equal(got, want) {
			t.Errorf("ECDSA signed algos = %v; want %v", got, want)
		}
	})

	t.Run("rsa_fallback", func(t *testing.T) {
		rsaSigner := newRecordingSigner(t, hostKey)

		connectWithHostKeys(t, rsaSigner)

		if got, want := rsaSigner.signedAlgos(), []string{cryptossh.KeyAlgoRSASHA256}; !slices.Equal(got, want) {
			t.Errorf("RSA signed algos = %v; want %v", got, want)
		}
	})

	t.Run("ed25519_cert_preferred", func(t *testing.T) {
		rsaSigner := newRecordingSigner(t, hostKey)
		ecdsaSigner := newRecordingSigner(t, ecdsaPriv)
		edSigner := newRecordingSigner(t, edPriv)

		// x/crypto/ssh defaults to CertAlgoRSASHA256v01 ahead of ECDSA and ED25519
		// certificates, and signs handshakes using the certificate's underlying key.
		connectWithHostKeys(t,
			newHostCertSigner(t, rsaSigner, caSigner),
			newHostCertSigner(t, ecdsaSigner, caSigner),
			newHostCertSigner(t, edSigner, caSigner),
		)

		if got := rsaSigner.signedAlgos(); len(got) != 0 {
			t.Errorf("RSA cert signed algos = %v; want []", got)
		}
		if got := ecdsaSigner.signedAlgos(); len(got) != 0 {
			t.Errorf("ECDSA cert signed algos = %v; want []", got)
		}
		if got, want := edSigner.signedAlgos(), []string{cryptossh.KeyAlgoED25519}; !slices.Equal(got, want) {
			t.Errorf("ED25519 cert signed algos = %v; want %v", got, want)
		}
	})

	t.Run("ecdsa_cert_fallback", func(t *testing.T) {
		rsaSigner := newRecordingSigner(t, hostKey)
		ecdsaSigner := newRecordingSigner(t, ecdsaPriv)

		connectWithHostKeys(t,
			newHostCertSigner(t, rsaSigner, caSigner),
			newHostCertSigner(t, ecdsaSigner, caSigner),
		)

		if got := rsaSigner.signedAlgos(); len(got) != 0 {
			t.Errorf("RSA cert signed algos = %v; want []", got)
		}
		if got, want := ecdsaSigner.signedAlgos(), []string{cryptossh.KeyAlgoECDSA256}; !slices.Equal(got, want) {
			t.Errorf("ECDSA cert signed algos = %v; want %v", got, want)
		}
	})
}
