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
	"sort"
	"strconv"
	"strings"
	"syscall"
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

// SSH represents an SSH connection to another computer.
type SSH struct {
	cl *ssh.Client

	// platform describes the Operating System running on the remote computer. Guaranteed to
	// be non-nil.
	platform *Platform

	// AnnounceCmd (if non-nil) is called with every remote command immediately before it's executed.
	// This is useful for testing (i.e. to ensure that only expected commands are executed).
	AnnounceCmd func(string)
}

// SSHOptions contains options used when connecting to an SSH server.
type SSHOptions struct {
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
// questionPrefix is used to prompt for a password when using keyboard-interactive authentication.
func getSSHAuthMethods(o *SSHOptions, questionPrefix string) ([]ssh.AuthMethod, error) {
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

// NewSSH establishes an SSH connection to the host described in o.
// Callers are responsible to call SSH.Close after using it.
func NewSSH(ctx context.Context, o *SSHOptions) (*SSH, error) {
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
			return &SSH{cl, o.Platform, nil}, nil
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
func (s *SSH) Close(ctx context.Context) error {
	return doAsync(ctx, func() error { return s.cl.Conn.Close() }, nil)
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
	rcmd := s.Command("tar", "-c", "--gzip", "-C", filepath.Dir(src), sb)
	p, err := rcmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %v", err)
	}
	if err := rcmd.Start(ctx); err != nil {
		return fmt.Errorf("running remote tar failed: %v", err)
	}
	defer rcmd.Wait(ctx)
	defer rcmd.Abort()

	cmd := exec.CommandContext(ctx, "/bin/tar", "-x", "--gzip", "--no-same-owner", "-C", td)
	cmd.Stdin = p
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running local tar failed: %v", err)
	}

	if err := os.Rename(filepath.Join(td, sb), dst); err != nil {
		return fmt.Errorf("moving local file failed: %v", err)
	}
	return nil
}

// SymlinkPolicy describes how symbolic links should be handled by PutFiles.
type SymlinkPolicy int

const (
	// PreserveSymlinks indicates that symlinks should be preserved during the copy.
	PreserveSymlinks SymlinkPolicy = iota
	// DereferenceSymlinks indicates that symlinks should be dereferenced and turned into normal files.
	DereferenceSymlinks
)

// countingReader is an io.Reader wrapper that counts the transferred bytes.
type countingReader struct {
	r     io.Reader
	bytes int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	c, err := r.r.Read(p)
	r.bytes += int64(c)
	return c, err
}

// PutFiles copies files on the local machine to the host. files describes
// a mapping from a local file path to a remote file path. For example, the call:
//
//	PutFiles(ctx, map[string]string{"/src/from": "/dst/to"})
//
// will copy the local file or directory /src/from to /dst/to on the remote host.
// Local file paths can be absolute or relative. Remote file paths must be absolute.
// SHA1 hashes of remote files are checked in advance to send updated files only.
// bytes is the amount of data sent over the wire (possibly after compression).
func (s *SSH) PutFiles(ctx context.Context, files map[string]string,
	symlinkPolicy SymlinkPolicy) (bytes int64, err error) {
	af := make(map[string]string)
	for src, dst := range files {
		if !filepath.IsAbs(src) {
			p, err := filepath.Abs(src)
			if err != nil {
				return 0, fmt.Errorf("source path %q could not be resolved", src)
			}
			src = p
		}
		if !filepath.IsAbs(dst) {
			return 0, fmt.Errorf("destination path %q should be absolute", dst)
		}
		af[src] = dst
	}

	// TODO(derat): When copying a small amount of data, it may be faster to avoid the extra
	// comparison round trip(s) and instead just copy unconditionally.
	cf, err := s.findChangedFiles(ctx, af)
	if err != nil {
		return 0, err
	}
	if len(cf) == 0 {
		return 0, nil
	}

	args := []string{"-c", "--gzip", "-C", "/"}
	if symlinkPolicy == DereferenceSymlinks {
		args = append(args, "--dereference")
	}
	for l, r := range cf {
		args = append(args, tarTransformFlag(strings.TrimPrefix(l, "/"), strings.TrimPrefix(r, "/")))
	}
	for l := range cf {
		args = append(args, strings.TrimPrefix(l, "/"))
	}
	cmd := exec.CommandContext(ctx, "/bin/tar", args...)
	p, err := cmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to open stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("running local tar failed: %v", err)
	}
	defer cmd.Wait()
	defer syscall.Kill(cmd.Process.Pid, syscall.SIGKILL)

	rcmd := s.Command("tar", "-x", "--gzip", "--no-same-owner", "--recursive-unlink", "-C", "/")
	cr := &countingReader{r: p}
	rcmd.Stdin = cr
	if err := rcmd.Run(ctx); err != nil {
		return 0, fmt.Errorf("remote tar failed: %v", err)
	}
	return cr.bytes, nil
}

// tarTransformFlag returns a GNU tar --transform flag for renaming path s to d when
// creating an archive.
func tarTransformFlag(s, d string) string {
	esc := func(s string, bad []string) string {
		for _, b := range bad {
			s = strings.Replace(s, b, "\\"+b, -1)
		}
		return s
	}
	return fmt.Sprintf("--transform=s,^%s$,%s,",
		esc(regexp.QuoteMeta(s), []string{","}),
		esc(d, []string{"\\", ",", "&"}))
}

// findChangedFiles returns a subset of files that differ between the local machine
// and the remote machine. This function is intended for use when pushing files to s;
// an error is returned if one or more files are missing locally, but not if they're
// only missing remotely. Local directories are always listed as having been changed.
func (s *SSH) findChangedFiles(ctx context.Context, files map[string]string) (map[string]string, error) {
	if len(files) == 0 {
		return nil, nil
	}

	// Sort local names.
	lp := make([]string, 0, len(files))
	for l := range files {
		lp = append(lp, l)
	}
	sort.Strings(lp)

	// TODO(derat): For large binary files, it may be faster to do an extra round trip first
	// to get file sizes. If they're different, there's no need to spend the time and
	// CPU to run sha1sum.
	rp := make([]string, len(lp))
	for i, l := range lp {
		rp[i] = files[l]
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

	cf := make(map[string]string)
	for i, l := range lp {
		r := rp[i]
		// TODO(derat): Also check modes, maybe.
		if lh[l] != rh[r] {
			cf[l] = r
		}
	}
	return cf, nil
}

// getRemoteSHA1s returns SHA1s for the files paths on s.
// Missing files are excluded from the returned map.
func (s *SSH) getRemoteSHA1s(ctx context.Context, paths []string) (map[string]string, error) {
	out, err := s.Command("sha1sum", paths...).Output(ctx)
	if err != nil {
		// TODO(derat): Find a classier way to ignore missing files.
		if _, ok := err.(*ssh.ExitError); !ok {
			return nil, fmt.Errorf("failed to hash files: %v", err)
		}
	}

	sums := make(map[string]string, len(paths))
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
	sums := make(map[string]string, len(paths))

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

// DeleteTree deletes all relative paths in files from baseDir on the host.
// If a specified file is a directory, all files under it are recursively deleted.
// Non-existent files are ignored.
func (s *SSH) DeleteTree(ctx context.Context, baseDir string, files []string) error {
	var cfs []string
	for _, f := range files {
		cf, err := cleanRelativePath(f)
		if err != nil {
			return err
		}
		cfs = append(cfs, cf)
	}

	cmd := s.Command("rm", append([]string{"-rf", "--"}, cfs...)...)
	cmd.Dir = baseDir
	if err := cmd.Run(ctx); err != nil {
		return fmt.Errorf("running remote rm failed: %v", err)
	}
	return nil
}

// cleanRelativePath ensures p is a relative path not escaping the base directory and
// returns a path cleaned by filepath.Clean.
func cleanRelativePath(p string) (string, error) {
	cp := filepath.Clean(p)
	if filepath.IsAbs(cp) {
		return "", fmt.Errorf("%s is an absolute path", p)
	}
	if strings.HasPrefix(cp, "../") {
		return "", fmt.Errorf("%s escapes the base directory", p)
	}
	return cp, nil
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

// ListenTCP opens a remote TCP port for listening.
func (s *SSH) ListenTCP(addr *net.TCPAddr) (net.Listener, error) {
	return s.cl.ListenTCP(addr)
}

// NewForwarder creates a new Forwarder that forwards connections from localAddr to remoteAddr using s.
// localAddr is passed to net.Listen and typically takes the form "host:port" or "ip:port".
// remoteAddr uses the same format but is resolved by the remote SSH server.
// If non-nil, errFunc will be invoked asynchronously on a goroutine with connection or forwarding errors.
func (s *SSH) NewForwarder(localAddr, remoteAddr string, errFunc func(error)) (*Forwarder, error) {
	connFunc := func() (net.Conn, error) { return s.cl.Dial("tcp", remoteAddr) }
	return newForwarder(localAddr, connFunc, errFunc)
}
