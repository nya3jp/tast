// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package host

import (
	"context"
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

// sshHost represents an SSH connection to another computer.
type sshHost struct {
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

	// AnnounceCmd (if non-nil) is passed every remote command immediately before it's executed.
	// This is useful for testing (i.e. to ensure that only expected commands are executed).
	AnnounceCmd func(string)
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
	methods = append(methods, ssh.KeyboardInteractive(
		func(user, inst string, qs []string, es []bool) (as []string, err error) {
			as = make([]string, len(qs))
			for i, q := range qs {
				// Print a prefix before the question to make it less likely the user
				// automatically types their own password since they're used to being
				// prompted by sudo whenever they run a command. :-/
				os.Stdout.WriteString(questionPrefix + q)
				b, err := terminal.ReadPassword(int(os.Stdin.Fd()))
				os.Stdout.WriteString("\n")
				if err != nil {
					return nil, err
				}
				as[i] = string(b)
			}
			return as, nil
		}))

	return methods, nil
}

// NewSSH establishes an SSH-based connection to the host described in o.
func NewSSH(ctx context.Context, o *SSHOptions) (Host, error) {
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
		return &sshHost{cl, o.AnnounceCmd}, nil
	case err = <-ech:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (h *sshHost) Close(ctx context.Context) error {
	ch := make(chan error, 1)
	go func() {
		ch <- h.cl.Conn.Close()
	}()

	select {
	case err := <-ch:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *sshHost) GetFile(ctx context.Context, src, dst string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	// Check that the dest path doesn't already exist to prevent people from
	// shooting themselves in the foot by supplying a destination directory
	// instead of the actual destination path for the copied file.
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("dest %q already exists", dst)
	}

	// Create a temporary directory alongside the destination path.
	td, err := ioutil.TempDir(filepath.Dir(dst), filepath.Base(dst)+".")
	if err != nil {
		return fmt.Errorf("creating local temp dir failed: %v", err)
	}
	defer os.RemoveAll(td)

	sb := filepath.Base(src)
	rc := fmt.Sprintf("tar -c --gzip -C %s %s", QuoteShellArg(filepath.Dir(src)), QuoteShellArg(sb))
	handle, err := h.Start(ctx, rc, CloseStdin, StdoutOnly)
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

func (h *sshHost) PutFile(ctx context.Context, src, dst string) (bytes int64, err error) {
	// Things are easy if we're using the same base filename on both ends.
	if filepath.Base(src) == filepath.Base(dst) {
		return h.PutTree(ctx, filepath.Dir(src), filepath.Dir(dst), []string{filepath.Base(src)})
	}

	// Otherwise, create a temp dir on the remote host, copy the file into it, and then
	// rename the file.
	b, err := h.Run(ctx, fmt.Sprintf("mkdir -p %s && mktemp -d %s.XXXXXXXXXX",
		QuoteShellArg(filepath.Dir(dst)), QuoteShellArg(dst)))
	if err != nil {
		return 0, fmt.Errorf("creating remote temp dir failed: %v", err)
	}
	td := strings.TrimSuffix(string(b), "\n")

	if bytes, err = h.PutTree(ctx, filepath.Dir(src), td, []string{filepath.Base(src)}); err != nil {
		h.Run(ctx, fmt.Sprintf("rm -rf %s", QuoteShellArg(td)))
		return 0, err
	}
	cmd := fmt.Sprintf("rm -rf %s && mv %s %s && rmdir %s", QuoteShellArg(dst),
		QuoteShellArg(filepath.Join(td, filepath.Base(src))), QuoteShellArg(dst),
		QuoteShellArg(td))
	if _, err := h.Run(ctx, cmd); err != nil {
		return 0, fmt.Errorf("renaming remote file failed: %v", err)
	}
	return bytes, nil
}

func (h *sshHost) PutTree(ctx context.Context, srcDir, dstDir string, files []string) (bytes int64, err error) {
	// TODO(derat): When copying a small amount of data, it may be faster to avoid the extra
	// comparison round trip(s) and instead just copy unconditionally.
	cf, err := findChangedFiles(ctx, h, srcDir, dstDir, files)
	if err != nil {
		return 0, err
	}
	if len(cf) == 0 {
		return 0, nil
	}

	qd := QuoteShellArg(dstDir)
	rc := fmt.Sprintf("mkdir -p %s && tar -x --gzip --no-same-owner -C %s 2>&1", qd, qd)
	handle, err := h.Start(ctx, rc, OpenStdin, StdoutOnly)
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

// bytesAndError wraps a byte slice and error so they can be passed together over a channel.
type bytesAndError struct {
	b   []byte
	err error
}

func (h *sshHost) Run(ctx context.Context, cmd string) ([]byte, error) {
	if h.announceCmd != nil {
		h.announceCmd(cmd)
	}

	ch := make(chan bytesAndError, 1)
	go func() {
		session, err := h.cl.NewSession()
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

func (h *sshHost) Start(ctx context.Context, cmd string, input InputMode, output OutputMode) (CommandHandle, error) {
	// TODO(derat): Watch ctx.Done() when running blocking commands.
	var err error
	c := &sshCommandHandle{}
	c.session, err = h.cl.NewSession()
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

	if h.announceCmd != nil {
		h.announceCmd(cmd)
	}
	if err = c.session.Start(cmd); err != nil {
		c.session.Close()
		return nil, fmt.Errorf("failed to start: %v", err)
	}
	return c, nil
}

func (h *sshHost) Ping(ctx context.Context, timeout time.Duration) error {
	ch := make(chan error, 1)
	go func() {
		_, _, err := h.cl.SendRequest(sshMsgIgnore, true, []byte{})
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

// sshCommandHandle implements CommandHandle and represents an in-progress remote command.
type sshCommandHandle struct {
	session        *ssh.Session
	stderr, stdout io.Reader
	stdin          io.WriteCloser
}

func (c *sshCommandHandle) Close(ctx context.Context) error {
	// TODO(derat): Watch ctx.Done().
	err := c.session.Close()
	if err == io.EOF {
		err = nil
	}
	return err
}

func (c *sshCommandHandle) Stderr() io.Reader {
	return c.stderr
}

func (c *sshCommandHandle) Stdin() io.WriteCloser {
	return c.stdin
}

func (c *sshCommandHandle) Stdout() io.Reader {
	return c.stdout
}

func (c *sshCommandHandle) Wait(ctx context.Context) error {
	// TODO(derat): Watch ctx.Done().
	return c.session.Wait()
}
