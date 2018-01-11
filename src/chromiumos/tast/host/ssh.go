// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package host implements communication with remote hosts.
package host

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/terminal"
)

// InputMode describes how stdin should be handled when running a remote command.
// Commands may block if stdin is never closed.
type InputMode int

const (
	// OpenStdin indicates that stdin should be copied to the remote command.
	OpenStdin InputMode = iota
	// CloseStdin indicates that stdin should be closed.
	CloseStdin
)

// OutputMode describes how stdout and stderr should be handled when running a remote command.
// Commands may block if output is not consumed.
type OutputMode int

const (
	// StdoutAndStderr indicates that stdout and stderr should both be returned separately.
	StdoutAndStderr OutputMode = iota
	// Stdout indicates that only stdout should be returned (i.e. stderr should be closed).
	StdoutOnly
	// Stderr indicates that only stderr should be returned (i.e. stdout should be closed).
	StderrOnly
	// NoOutput indicates that both stdout and stderr should be closed.
	NoOutput
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

// SSH represents an SSH connection to another computer.
type SSH struct {
	cl          *ssh.Client
	announceCmd func(string) // See SSHOptions.AnnounceCmd.
}

// SSHOptions contains options used when connecting to an SSH server.
type SSHOptions struct {
	// User is the username to user when connecting.
	User string
	// Hostname is the SSH server's hostname.
	Hostname string
	// Port is the SSH server's TCP port.
	Port int

	// KeyPath is an optional path to an unencrypted SSH private key.
	KeyPath string

	// ConnectTimeout contains a timeout for establishing the TCP connection.
	ConnectTimeout time.Duration

	// AnnounceCmd (if non-nil) is passed to every remote command immediately before it's executed.
	// This is useful for testing (i.e. to ensure that only expected commands are executed).
	AnnounceCmd func(string)
}

// QuoteShellArg returns a single-quoted copy of s that can be inserted into command lines interpreted by sh.
func QuoteShellArg(s string) string {
	s = strings.Replace(s, "'", "'\"'\"'", -1)
	return "'" + s + "'"
}

// ParseSSHTarget parses target (of the form "[<user>@]host[:<port>]") and fills
// the User, Hostname, and Port fields in o, using reasonable defaults for unspecified values.
func ParseSSHTarget(target string, o *SSHOptions) error {
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
// keyPath may contain the path to an unencrypted private key, while questionPrefix is used to
// prompt for a password when using keyboard-interactive authentication.
func getSSHAuthMethods(keyPath, questionPrefix string) ([]ssh.AuthMethod, error) {
	methods := make([]ssh.AuthMethod, 0)

	// Load the private key if it was supplied.
	if keyPath != "" {
		k, err := ioutil.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key %s: %v", keyPath, err)
		}
		s, err := ssh.ParsePrivateKey(k)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key %s: %v", keyPath, err)
		}
		methods = append(methods, ssh.PublicKeys(s))
	}

	// Connect to ssh-agent if it's running.
	if s := os.Getenv("SSH_AUTH_SOCK"); s != "" {
		// TODO(derat): Sigh, $SSH_AUTH_SOCK appears to frequently be hosed in chroots that are
		// running under screen or tmux. Consider logging errors somewhere.
		if a, err := net.Dial("unix", s); err == nil {
			methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(a).Signers))
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

// NewSSH establishes an SSH-based connection to the host described in o.
func NewSSH(ctx context.Context, o *SSHOptions) (*SSH, error) {
	am, err := getSSHAuthMethods(o.KeyPath, "["+o.Hostname+"] ")
	if err != nil {
		return nil, err
	}
	cfg := ssh.ClientConfig{
		User:            o.User,
		Auth:            am,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	cfg.Timeout = o.ConnectTimeout

	cch := make(chan *ssh.Client, 1)
	ech := make(chan error, 1)
	go func() {
		cl, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", o.Hostname, o.Port), &cfg)
		if err != nil {
			ech <- err
		} else {
			cch <- cl
		}
	}()

	select {
	case cl := <-cch:
		return &SSH{cl, o.AnnounceCmd}, nil
	case err = <-ech:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close closes the underlying connection to the host.
func (s *SSH) Close(ctx context.Context) error {
	ch := make(chan error, 1)
	go func() {
		ch <- s.cl.Conn.Close()
	}()

	select {
	case err := <-ch:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// GetFile copies a file or directory from the host to the local machine.
// dst is the full destination name for the file or directory being copied, not
// a destination directory into which it will be copied. dst will be replaced
// if it already exists.
func (s *SSH) GetFile(ctx context.Context, src, dst string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	// Create a temporary directory alongside the destination path.
	td, err := ioutil.TempDir(filepath.Dir(dst), filepath.Base(dst)+".")
	if err != nil {
		return fmt.Errorf("creating local temp dir failed: %v", err)
	}
	defer os.RemoveAll(td)

	sb := filepath.Base(src)
	rc := fmt.Sprintf("tar -c --gzip -C %s %s", QuoteShellArg(filepath.Dir(src)), QuoteShellArg(sb))
	handle, err := s.Start(ctx, rc, CloseStdin, StdoutOnly)
	if err != nil {
		return fmt.Errorf("running remote tar failed: %v", err)
	}
	defer handle.Close(ctx)

	cmd := exec.Command("/bin/tar", "-x", "--gzip", "--no-same-owner", "-C", td)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("getting stdin for local tar failed: %v", err)
	}
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("running local tar failed: %v", err)
	}

	// TODO(derat): Watch ctx.Done() while streaming data.
	if _, err = io.Copy(stdin, handle.Stdout()); err != nil {
		return fmt.Errorf("copying from remote to local tar failed: %v", err)
	}
	if err = stdin.Close(); err != nil {
		return fmt.Errorf("closing local tar failed: %v", err)
	}
	if err = cmd.Wait(); err != nil {
		return fmt.Errorf("local tar failed: %v", err)
	}
	if err = os.Rename(filepath.Join(td, sb), dst); err != nil {
		return fmt.Errorf("moving local file failed: %v", err)
	}
	return nil
}

// PutTree copies all relative paths in files from srcDir on the local machine
// to dstDir on the host. For example, the call:
//
//	PutTree("/src", "/dst", []string{"a", "dir/b"})
//
// will result in the local file or directory /src/a being copied to /dst/a on
// the remote host and /src/dir/b being copied to /dst/dir/b. Existing files or directories
// within dstDir with names listed in files will be overwritten. bytes is the amount of data
// sent over the wire (possibly after compression).
func (s *SSH) PutTree(ctx context.Context, srcDir, dstDir string, files []string) (bytes int64, err error) {
	// TODO(derat): When copying a small amount of data, it may be faster to avoid the extra
	// comparison round trip(s) and instead just copy unconditionally.
	cf, err := s.findChangedFiles(ctx, srcDir, dstDir, files)
	if err != nil {
		return 0, err
	}
	if len(cf) == 0 {
		return 0, nil
	}

	qd := QuoteShellArg(dstDir)
	rc := fmt.Sprintf("mkdir -p %s && "+
		"tar -x --gzip --no-same-owner --recursive-unlink -C %s 2>&1", qd, qd)
	handle, err := s.Start(ctx, rc, OpenStdin, StdoutOnly)
	if err != nil {
		return 0, fmt.Errorf("running remote command %q failed: %v", rc, err)
	}
	defer handle.Close(ctx)

	args := []string{"-c", "--gzip", "-C", srcDir}
	args = append(args, cf...)
	cmd := exec.Command("/bin/tar", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("getting stdout for local tar failed: %v", err)
	}
	if err = cmd.Start(); err != nil {
		return 0, fmt.Errorf("running local tar failed: %v", err)
	}

	if bytes, err = io.Copy(handle.Stdin(), stdout); err != nil {
		return 0, fmt.Errorf("copying from local to remote tar failed: %v", err)
	}

	if err = cmd.Wait(); err != nil {
		return 0, fmt.Errorf("local tar failed: %v", err)
	}
	if err = handle.Stdin().Close(); err != nil {
		return 0, fmt.Errorf("closing remote tar failed: %v", err)
	}
	if err = handle.Wait(ctx); err != nil {
		return 0, fmt.Errorf("remote tar failed: %v", err)
	}
	return bytes, nil
}

// findChangedFiles returns paths from files that differ between ldir on the local
// machine and rdir on s. This function is intended for use when pushing files to h;
// an error is returned if one or more files are missing locally, but not if they're
// only missing remotely. Local directories are always listed as having been changed.
func (s *SSH) findChangedFiles(ctx context.Context, ldir, rdir string, files []string) ([]string, error) {
	if len(files) == 0 {
		return []string{}, nil
	}

	// TODO(derat): For large binary files, it may be faster to do an extra round trip first
	// to get file sizes. If they're different, there's no need to spend the time and
	// CPU to run sha1sum.
	lp := make([]string, len(files))
	rp := make([]string, len(files))
	for i, f := range files {
		lp[i] = filepath.Join(ldir, f)
		rp[i] = filepath.Join(rdir, f)
	}

	var lh, rh map[string]string
	ch := make(chan error, 2)
	go func() {
		var err error
		lh, err = getLocalSHA1s(lp)
		ch <- err
	}()
	go func() {
		var err error
		rh, err = s.getRemoteSHA1s(ctx, rp)
		ch <- err
	}()
	for i := 0; i < 2; i++ {
		if err := <-ch; err != nil {
			return nil, fmt.Errorf("failed to get SHA1(s): %v", err)
		}
	}

	cf := make([]string, 0)
	for i, f := range files {
		// TODO(derat): Also check modes, maybe.
		if lh[lp[i]] != rh[rp[i]] {
			cf = append(cf, f)
		}
	}
	return cf, nil
}

// getRemoteSHA1s returns SHA1s for the files paths on s.
// Missing files are excluded from the returned map.
func (s *SSH) getRemoteSHA1s(ctx context.Context, paths []string) (map[string]string, error) {
	cmd := "sha1sum"
	for _, p := range paths {
		cmd += " " + QuoteShellArg(p)
	}
	// TODO(derat): Find a classier way to ignore missing files.
	cmd += " 2>/dev/null || true"

	out, err := s.Run(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to hash files: %v", err)
	}

	sums := make(map[string]string)
	for _, l := range strings.Split(string(out), "\n") {
		if l == "" {
			continue
		}
		f := strings.Fields(l)
		if len(f) != 2 {
			return nil, fmt.Errorf("unexpected line %q from sha1sum", l)
		}
		sums[f[1]] = f[0]
	}
	return sums, nil
}

// getLocalSHA1s returns SHA1s for files in paths.
// An error is returned if any files are missing.
func getLocalSHA1s(paths []string) (map[string]string, error) {
	sums := make(map[string]string)

	for _, p := range paths {
		if fi, err := os.Stat(p); err != nil {
			return nil, err
		} else if fi.IsDir() {
			// Use a bogus hash for directories to ensure they're copied.
			sums[p] = "dir-hash"
			continue
		}

		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		h := sha1.New()
		if _, err := io.Copy(h, f); err != nil {
			return nil, err
		}
		sums[p] = hex.EncodeToString(h.Sum(nil))
	}

	return sums, nil
}

// bytesAndError wraps a byte slice and error so they can be passed together over a channel.
type bytesAndError struct {
	b   []byte
	err error
}

// Run runs cmd synchronously on the host and returns its output. stdout and stderr are combined.
// cmd is interpreted by the user's shell; arguments may be quoted using QuoteShellArg.
// If the command is interrupted or exits with a nonzero status code, the returned error will
// be of type *ssh.ExitError.
func (s *SSH) Run(ctx context.Context, cmd string) ([]byte, error) {
	if s.announceCmd != nil {
		s.announceCmd(cmd)
	}

	ch := make(chan bytesAndError, 1)
	go func() {
		session, err := s.cl.NewSession()
		if err != nil {
			ch <- bytesAndError{nil, fmt.Errorf("failed to create session: %v", err)}
			return
		}
		defer session.Close()
		b, err := session.CombinedOutput(cmd)
		ch <- bytesAndError{b, err}
	}()

	select {
	case be := <-ch:
		return be.b, be.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Start runs cmd asynchronously on the host and returns a handle that can be used to write input,
// read output, and wait for completion. cmd is interpreted by the user's shell; arguments may be
// quoted using QuoteShellArg.
func (s *SSH) Start(ctx context.Context, cmd string, input InputMode, output OutputMode) (*SSHCommandHandle, error) {
	// TODO(derat): Watch ctx.Done() when running blocking commands.
	var err error
	c := &SSHCommandHandle{}
	c.session, err = s.cl.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %v", err)
	}

	if output == StdoutAndStderr || output == StdoutOnly {
		if c.stdout, err = c.session.StdoutPipe(); err != nil {
			c.session.Close()
			return nil, fmt.Errorf("failed to get stdout: %v", err)
		}
	}
	if output == StdoutAndStderr || output == StderrOnly {
		if c.stderr, err = c.session.StderrPipe(); err != nil {
			c.session.Close()
			return nil, fmt.Errorf("failed to get stderr: %v", err)
		}
	}

	if input == OpenStdin {
		if c.stdin, err = c.session.StdinPipe(); err != nil {
			c.session.Close()
			return nil, fmt.Errorf("failed to get stdin: %v", err)
		}
	}

	if s.announceCmd != nil {
		s.announceCmd(cmd)
	}
	if err = c.session.Start(cmd); err != nil {
		c.session.Close()
		return nil, fmt.Errorf("failed to start: %v", err)
	}
	return c, nil
}

// Ping checks that the connection to the host is still active, blocking until a
// response has been received. An error is returned if the connection is inactive or
// if timeout or ctx's deadline are exceeded.
func (s *SSH) Ping(ctx context.Context, timeout time.Duration) error {
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

// SSHCommandHandle represents an in-progress remote command.
type SSHCommandHandle struct {
	session        *ssh.Session
	stderr, stdout io.Reader
	stdin          io.WriteCloser
}

// Close closes the session in which the command is running.
func (h *SSHCommandHandle) Close(ctx context.Context) error {
	// TODO(derat): Watch ctx.Done().
	err := h.session.Close()
	if err == io.EOF {
		err = nil
	}
	return err
}

// Stderr returns a pipe connected to the command's stderr or nil if the OutputMode didn't include stderr.
func (h *SSHCommandHandle) Stderr() io.Reader {
	return h.stderr
}

// Stdin returns a pipe connected to the command's stdin or nil if the InputMode was not OpenStdin.
func (h *SSHCommandHandle) Stdin() io.WriteCloser {
	return h.stdin
}

// Stdout returns a pipe connected to the command's stdout or nil if the OutputMode didn't include stdin.
func (h *SSHCommandHandle) Stdout() io.Reader {
	return h.stdout
}

// Wait waits until the command finishes running.
func (h *SSHCommandHandle) Wait(ctx context.Context) error {
	// TODO(derat): Watch ctx.Done().
	return h.session.Wait()
}
