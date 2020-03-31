// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/terminal"
)

const (
	defaultSSHUser = "root"
	defaultSSHPort = 22

	// sshMsgIgnore is the SSH global message sent to ping the host.
	// See RFC 4253 11.2, "Ignored Data Message".
	sshMsgIgnore = "SSH_MSG_IGNORE"
)

// targetRegexp is used to parse targets passed to ParseSSHTarget.
var targetRegexp *regexp.Regexp

func init() {
	targetRegexp = regexp.MustCompile("^([^@]+@)?([^:@]+)(:\\d+)?$")
}

// Conn represents an SSH connection to another computer.
type Conn struct {
	cl *ssh.Client

	// platform describes the Operating System running on the remote computer. Guaranteed to
	// be non-nil.
	platform *Platform

	// AnnounceCmd (if non-nil) is called with every remote command immediately before it's executed.
	// This is useful for testing (i.e. to ensure that only expected commands are executed).
	AnnounceCmd func(string)
}

// Options contains options used when connecting to an SSH server.
type Options struct {
	// User is the username to user when connecting.
	User string
	// Hostname is the SSH server's hostname.
	Hostname string
	// Port is the SSH server's TCP port.
	Port int

	// KeyFile is an optional path to an unencrypted SSH private key.
	KeyFile string
	// KeyDir is an optional path to a directory (typically $HOME/.ssh) containing standard
	// SSH keys (id_dsa, id_rsa, etc.) to use if authentication via KeyFile is not accepted.
	// Only unencrypted keys are used.
	KeyDir string

	// ConnectTimeout contains a timeout for establishing the TCP connection.
	ConnectTimeout time.Duration
	// ConnectRetries contains the number of times to retry after a connection failure.
	// Each attempt waits up to ConnectTimeout.
	ConnectRetries int
	// ConnectRetryInterval contains the minimum amount of time between connection attempts.
	// This can be set to avoid quickly burning through all retries if errors are returned
	// immediately (e.g. connection refused while the SSH daemon is restarting).
	// The time spent trying to connect counts against this interval.
	ConnectRetryInterval time.Duration

	// WarnFunc (if non-nil) is used to log non-fatal errors encountered while connecting to the host.
	WarnFunc func(string)

	// Platform describes the operating system running on the SSH server. This controls how certain
	// commands will be executed on the remote system. If nil, assumes a Chrome OS system.
	Platform *Platform
}

// ParseTarget parses target (of the form "[<user>@]host[:<port>]") and fills
// the User, Hostname, and Port fields in o, using reasonable defaults for unspecified values.
func ParseTarget(target string, o *Options) error {
	m := targetRegexp.FindStringSubmatch(target)
	if m == nil {
		return fmt.Errorf("couldn't parse %q as \"[user@]hostname[:port]\"", target)
	}

	o.User = defaultSSHUser
	if m[1] != "" {
		o.User = m[1][0 : len(m[1])-1]
	}
	o.Hostname = m[2]
	o.Port = defaultSSHPort
	if m[3] != "" {
		s := m[3][1:]
		p, err := strconv.ParseInt(s, 10, 32)
		if err != nil || p <= 0 || p > 65535 {
			return fmt.Errorf("invalid port %q", s)
		}
		o.Port = int(p)
	}

	return nil
}

// getSSHAuthMethods returns authentication methods to use when connecting to a remote server.
// questionPrefix is used to prompt for a password when using keyboard-interactive authentication.
func getSSHAuthMethods(o *Options, questionPrefix string) ([]ssh.AuthMethod, error) {
	methods := make([]ssh.AuthMethod, 0)

	// Start with SSH keys.
	keySigners := make([]ssh.Signer, 0)
	if o.KeyFile != "" {
		s, _, err := readPrivateKey(o.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key %s: %v", o.KeyFile, err)
		}
		keySigners = append(keySigners, s)
	}
	if o.KeyDir != "" {
		// testing_rsa is used by Autotest's SSH config, so look for the same key here.
		// See https://www.chromium.org/chromium-os/testing/autotest-developer-faq/ssh-test-keys-setup.
		// mobbase_id_rsa is stored in /home/moblab/.ssh on Moblab devices.
		for _, fn := range []string{"testing_rsa", "mobbase_id_rsa", "id_dsa", "id_ecdsa", "id_ed25519", "id_rsa"} {
			p := filepath.Join(o.KeyDir, fn)
			if p == o.KeyFile {
				continue
			} else if _, err := os.Stat(p); os.IsNotExist(err) {
				continue
			}
			if s, rok, err := readPrivateKey(p); err == nil {
				keySigners = append(keySigners, s)
			} else if !rok && o.WarnFunc != nil {
				o.WarnFunc(fmt.Sprintf("Failed to read %v: %v", p, err))
			}
		}
	}
	if len(keySigners) > 0 {
		methods = append(methods, ssh.PublicKeys(keySigners...))
	}

	// Connect to ssh-agent if it's running.
	if s := os.Getenv("SSH_AUTH_SOCK"); s != "" {
		if a, err := net.Dial("unix", s); err == nil {
			methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(a).Signers))
		} else if o.WarnFunc != nil {
			o.WarnFunc(fmt.Sprintf("Failed to connect to ssh-agent at %v: %v", s, err))
		}
	}

	// Fall back to keyboard-interactive.
	stdin := int(os.Stdin.Fd())
	if terminal.IsTerminal(stdin) {
		methods = append(methods, ssh.KeyboardInteractive(
			func(user, inst string, qs []string, es []bool) (as []string, err error) {
				return presentChallenges(stdin, questionPrefix, user, inst, qs, es)
			}))
	}

	return methods, nil
}

// readPrivateKey reads and decodes a passphraseless private SSH key from path.
// rok is true if the key data was read successfully off disk and false if it wasn't.
// Note that err may be set while rok is true if the key was malformed or passphrase-protected.
func readPrivateKey(path string) (s ssh.Signer, rok bool, err error) {
	k, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	s, err = ssh.ParsePrivateKey(k)
	return s, true, err
}

// presentChallenges prints the challenges in qs and returns the user's answers.
// This (minus its additional two first arguments) is an ssh.KeyboardInteractiveChallenge
// suitable for passing to ssh.AuthMethod.KeyboardInteractive.
func presentChallenges(stdin int, prefix, user, inst string, qs []string, es []bool) (
	as []string, err error) {
	as = make([]string, len(qs))
	for i, q := range qs {
		// Print a prefix before the question to make it less likely the user
		// automatically types their own password since they're used to being
		// prompted by sudo whenever they run a command. :-/
		os.Stdout.WriteString(prefix + q)
		b, err := terminal.ReadPassword(stdin)
		os.Stdout.WriteString("\n")
		if err != nil {
			return nil, err
		}
		as[i] = string(b)
	}
	return as, nil
}

// New establishes an SSH connection to the host described in o.
// Callers are responsible to call Conn.Close after using it.
func New(ctx context.Context, o *Options) (*Conn, error) {
	if o.Port == 0 {
		o.Port = defaultSSHPort
	}
	if o.User == "" {
		o.User = defaultSSHUser
	}
	if o.Platform == nil {
		o.Platform = DefaultPlatform
	}

	hostPort := fmt.Sprintf("%s:%d", o.Hostname, o.Port)
	am, err := getSSHAuthMethods(o, "["+o.Hostname+"] ")
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{
		User:            o.User,
		Auth:            am,
		Timeout:         o.ConnectTimeout,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	for i := 0; i < o.ConnectRetries+1; i++ {
		start := time.Now()
		var cl *ssh.Client
		if cl, err = connectSSH(ctx, hostPort, cfg); err == nil {
			return &Conn{cl, o.Platform, nil}, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if i < o.ConnectRetries {
			elapsed := time.Now().Sub(start)
			if remaining := o.ConnectRetryInterval - elapsed; remaining > 0 {
				if o.WarnFunc != nil {
					o.WarnFunc(fmt.Sprintf("Retrying SSH connection in %v: %v", remaining.Round(time.Millisecond), err))
				}
				select {
				case <-time.After(remaining):
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			} else if o.WarnFunc != nil {
				o.WarnFunc(fmt.Sprintf("Retrying SSH connection: %v", err))
			}
		}
	}
	return nil, err
}

// connectSSH attempts to synchronously connect to hostPort as directed by cfg.
func connectSSH(ctx context.Context, hostPort string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
	var cl *ssh.Client
	if err := doAsync(ctx, func() error {
		var err error
		cl, err = ssh.Dial("tcp", hostPort, cfg)
		return err
	}, func() {
		if cl != nil {
			cl.Conn.Close()
		}
	}); err != nil {
		return nil, err
	}
	return cl, nil
}

// Close closes the underlying connection to the host.
func (s *Conn) Close(ctx context.Context) error {
	return doAsync(ctx, func() error { return s.cl.Conn.Close() }, nil)
}

// Ping checks that the connection to the host is still active, blocking until a
// response has been received. An error is returned if the connection is inactive or
// if timeout or ctx's deadline are exceeded.
func (s *Conn) Ping(ctx context.Context, timeout time.Duration) error {
	ch := make(chan error, 1)
	go func() {
		_, _, err := s.cl.SendRequest(sshMsgIgnore, true, []byte{})
		ch <- err
	}()

	select {
	case err := <-ch:
		return err
	case <-time.After(timeout):
		return errors.New("timed out")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ListenTCP opens a remote TCP port for listening.
func (s *Conn) ListenTCP(addr *net.TCPAddr) (net.Listener, error) {
	return s.cl.ListenTCP(addr)
}

// NewForwarder creates a new Forwarder that forwards connections from localAddr to remoteAddr using s.
// localAddr is passed to net.Listen and typically takes the form "host:port" or "ip:port".
// remoteAddr uses the same format but is resolved by the remote SSH server.
// If non-nil, errFunc will be invoked asynchronously on a goroutine with connection or forwarding errors.
func (s *Conn) NewForwarder(localAddr, remoteAddr string, errFunc func(error)) (*Forwarder, error) {
	connFunc := func() (net.Conn, error) { return s.cl.Dial("tcp", remoteAddr) }
	return newForwarder(localAddr, connFunc, errFunc)
}
