// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package linuxssh provides Linux specific operations conducted via SSH
package linuxssh

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	cryptossh "golang.org/x/crypto/ssh"
	"golang.org/x/sys/unix"

	"chromiumos/tast/errors"
	"chromiumos/tast/ssh"
)

// SymlinkPolicy describes how symbolic links should be handled by PutFiles.
type SymlinkPolicy int

const (
	// PreserveSymlinks indicates that symlinks should be preserved during the copy.
	PreserveSymlinks SymlinkPolicy = iota
	// DereferenceSymlinks indicates that symlinks should be dereferenced and turned into normal files.
	DereferenceSymlinks
)

// GetFile copies a file or directory from the host to the local machine.
// dst is the full destination name for the file or directory being copied, not
// a destination directory into which it will be copied. dst will be replaced
// if it already exists.
func GetFile(ctx context.Context, s *ssh.Conn, src, dst string, symlinkPolicy SymlinkPolicy) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	if err := os.RemoveAll(dst); err != nil {
		return err
	}

	path, close, err := getFile(ctx, s, src, dst, symlinkPolicy)
	if err != nil {
		return err
	}
	defer close()

	if err := os.Rename(path, dst); err != nil {
		return fmt.Errorf("moving local file failed: %v", err)
	}
	return nil
}

// getFile copies a file or directory from the host to the local machine.
// It creates a temporary directory under the directory of dst, and copies
// src to it. It returns the filepath where src has been copied to.
// Caller must call close to remove the temporary directory.
func getFile(ctx context.Context, s *ssh.Conn, src, dst string, symlinkPolicy SymlinkPolicy) (path string, close func() error, retErr error) {
	// Create a temporary directory alongside the destination path.
	td, err := ioutil.TempDir(filepath.Dir(dst), filepath.Base(dst)+".")
	if err != nil {
		return "", nil, fmt.Errorf("creating local temp dir failed: %v", err)
	}
	defer func() {
		if retErr != nil {
			os.RemoveAll(td)
		}
	}()
	close = func() error {
		return os.RemoveAll(td)
	}

	sb := filepath.Base(src)
	taropts := []string{"-c", "--gzip", "-C", filepath.Dir(src)}
	if symlinkPolicy == DereferenceSymlinks {
		taropts = append(taropts, "--dereference")
	}
	taropts = append(taropts, sb)
	rcmd := s.CommandContext(ctx, "tar", taropts...)
	p, err := rcmd.StdoutPipe()
	if err != nil {
		return "", nil, fmt.Errorf("failed to get stdout pipe: %v", err)
	}
	if err := rcmd.Start(); err != nil {
		return "", nil, fmt.Errorf("running remote tar failed: %v", err)
	}
	defer rcmd.Wait()
	defer rcmd.Abort()

	cmd := exec.CommandContext(ctx, "/bin/tar", "-x", "--gzip", "--no-same-owner", "-p", "-C", td)
	cmd.Stdin = p
	if err := cmd.Run(); err != nil {
		return "", nil, fmt.Errorf("running local tar failed: %v", err)
	}
	return filepath.Join(td, sb), close, nil
}

// findChangedFiles returns a subset of files that differ between the local machine
// and the remote machine. This function is intended for use when pushing files to s;
// an error is returned if one or more files are missing locally, but not if they're
// only missing remotely. Local directories are always listed as having been changed.
func findChangedFiles(ctx context.Context, s *ssh.Conn, files map[string]string) (map[string]string, error) {
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
		rh, err = getRemoteSHA1s(ctx, s, rp)
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
func getRemoteSHA1s(ctx context.Context, s *ssh.Conn, paths []string) (map[string]string, error) {
	out, err := s.CommandContext(ctx, "sha1sum", paths...).Output()
	if err != nil {
		// TODO(derat): Find a classier way to ignore missing files.
		if _, ok := err.(*cryptossh.ExitError); !ok {
			return nil, fmt.Errorf("failed to hash files: %v", err)
		}
	}

	sums := make(map[string]string, len(paths))
	for _, l := range strings.Split(string(out), "\n") {
		if l == "" {
			continue
		}
		f := strings.SplitN(l, " ", 2)
		if len(f) != 2 {
			return nil, fmt.Errorf("unexpected line %q from sha1sum", l)
		}
		if len(f[0]) != 40 {
			return nil, fmt.Errorf("invalid sha1 in line %q from sha1sum", l)
		}
		sums[strings.TrimLeft(f[1], " ")] = f[0]
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

// tarTransformFlag returns a GNU tar --transform flag for renaming path s to d when
// creating an archive.
func tarTransformFlag(s, d string) string {
	esc := func(s string, bad []string) string {
		for _, b := range bad {
			s = strings.Replace(s, b, "\\"+b, -1)
		}
		return s
	}
	// Transform foo -> bar but not foobar -> barbar. Therefore match foo$ or foo/
	return fmt.Sprintf(`--transform=s,^%s\($\|/\),%s,`,
		esc(regexp.QuoteMeta(s), []string{","}),
		esc(d, []string{"\\", ",", "&"}))
}

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
//	PutFiles(ctx, conn, map[string]string{"/src/from": "/dst/to"})
//
// will copy the local file or directory /src/from to /dst/to on the remote host.
// Local file paths can be absolute or relative. Remote file paths must be absolute.
// SHA1 hashes of remote files are checked in advance to send updated files only.
// bytes is the amount of data sent over the wire (possibly after compression).
func PutFiles(ctx context.Context, s *ssh.Conn, files map[string]string,
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
	cf, err := findChangedFiles(ctx, s, af)
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
	defer unix.Kill(cmd.Process.Pid, unix.SIGKILL)

	rcmd := s.CommandContext(ctx, "tar", "-x", "--gzip", "--no-same-owner", "--recursive-unlink", "-p", "-C", "/")
	cr := &countingReader{r: p}
	rcmd.Stdin = cr
	if err := rcmd.Run(ssh.DumpLogOnError); err != nil {
		return 0, fmt.Errorf("remote tar failed: %v", err)
	}
	return cr.bytes, nil
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

// DeleteTree deletes all relative paths in files from baseDir on the host.
// If a specified file is a directory, all files under it are recursively deleted.
// Non-existent files are ignored.
func DeleteTree(ctx context.Context, s *ssh.Conn, baseDir string, files []string) error {
	var cfs []string
	for _, f := range files {
		cf, err := cleanRelativePath(f)
		if err != nil {
			return err
		}
		cfs = append(cfs, cf)
	}

	cmd := s.CommandContext(ctx, "rm", append([]string{"-rf", "--"}, cfs...)...)
	cmd.Dir = baseDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running remote rm failed: %v", err)
	}
	return nil
}

// GetAndDeleteFile is similar to GetFile, but it also deletes a remote file
// when it is successfully copied.
func GetAndDeleteFile(ctx context.Context, s *ssh.Conn, src, dst string, policy SymlinkPolicy) error {
	if err := GetFile(ctx, s, src, dst, policy); err != nil {
		return err
	}
	if err := s.CommandContext(ctx, "rm", "-rf", "--", src).Run(); err != nil {
		return errors.Wrap(err, "delete failed")
	}
	return nil
}

// GetAndDeleteFilesInDir copies all files in dst to src, assuming both
// dst and src are directories.
// It deletes the remote directory if all the files are successfully copied.
func GetAndDeleteFilesInDir(ctx context.Context, s *ssh.Conn, src, dst string, policy SymlinkPolicy) error {
	dir, close, err := getFile(ctx, s, src, dst, policy)
	if err != nil {
		return err
	}
	defer close()

	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		dstPath := filepath.Join(dst, strings.TrimPrefix(path, dir))
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return err
		}
		if err := os.Rename(path, dstPath); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	if err := s.CommandContext(ctx, "rm", "-rf", "--", src).Run(); err != nil {
		return errors.Wrap(err, "delete failed")
	}
	return nil
}
