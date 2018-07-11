// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package host

import (
	"context"
	"crypto/rsa"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"chromiumos/tast/host/test"
	"chromiumos/tast/testutil"
)

var userKey, hostKey *rsa.PrivateKey

func init() {
	userKey, hostKey = test.MustGenerateKeys()
}

// connectToServer establishes a connection to srv using key.
func connectToServer(ctx context.Context, srv *test.SSHServer, key *rsa.PrivateKey) (*SSH, error) {
	keyFile, err := test.WriteKey(key)
	if err != nil {
		return nil, err
	}
	defer os.Remove(keyFile)

	o := &SSHOptions{KeyFile: keyFile}
	if err = ParseSSHTarget(srv.Addr().String(), o); err != nil {
		return nil, err
	}
	s, err := NewSSH(ctx, o)
	if err != nil {
		return nil, err
	}
	s.AnnounceCmd = srv.NextCmd
	return s, nil
}

// testData wraps data common to all tests.
type testData struct {
	ctx context.Context
	srv *test.SSHServer
	hst *SSH
}

func newTestData(t *testing.T) *testData {
	ctx := context.Background()
	srv, err := test.NewSSHServer(&userKey.PublicKey, hostKey)
	if err != nil {
		t.Fatal(err)
	}

	hst, err := connectToServer(ctx, srv, userKey)
	if err != nil {
		srv.Close()
		t.Fatal(err)
	}

	return &testData{ctx, srv, hst}
}

func (td *testData) close() {
	td.srv.Close()
	td.hst.Close(td.ctx)
}

// initFileTest creates a temporary directory with a subdirectory containing files.
// The temp dir's and subdir's paths are returned.
func initFileTest(t *testing.T, files map[string]string) (tmpDir, srcDir string) {
	tmpDir, err := ioutil.TempDir("", "ssh_test.")
	if err != nil {
		t.Fatal(err)
	}

	srcDir = filepath.Join(tmpDir, "src")
	if err = os.Mkdir(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err = testutil.WriteFiles(srcDir, files); err != nil {
		t.Fatal(err)
	}
	return tmpDir, srcDir
}

// checkFile returns an error if p's contents differ from exp.
func checkFile(p string, exp string) error {
	if b, err := ioutil.ReadFile(p); err != nil {
		return fmt.Errorf("failed to read %v after copy: %v", p, err)
	} else if string(b) != exp {
		return fmt.Errorf("expected content %q after copy, got %v", exp, string(b))
	}
	return nil
}

// checkDir returns an error if dir's contents don't match exp, a map from paths to data.
func checkDir(dir string, exp map[string]string) error {
	if act, err := testutil.ReadFiles(dir); err != nil {
		return err
	} else if !reflect.DeepEqual(exp, act) {
		return fmt.Errorf("files not copied correctly: src %v, dst %v", exp, act)
	}
	return nil
}

func TestRun(t *testing.T) {
	td := newTestData(t)
	defer td.close()

	if b, err := td.hst.Run(td.ctx, "echo foo"); err != nil {
		t.Error(err)
	} else if string(b) != "foo\n" {
		t.Errorf("Unexpected output %q for stdout", string(b))
	}

	if b, err := td.hst.Run(td.ctx, "echo bar >&2"); err != nil {
		t.Error(err)
	} else if string(b) != "bar\n" {
		t.Errorf("Unexpected output %q for stderr", string(b))
	}

	if b, err := td.hst.Run(td.ctx, "echo foo; false"); err == nil {
		t.Errorf("Didn't get error for failing command")
	} else if string(b) != "foo\n" {
		t.Errorf("Unexpected output %q for failing command", string(b))
	}

	ctx, cancel := context.WithTimeout(td.ctx, 10*time.Millisecond)
	defer cancel()
	if _, err := td.hst.Run(ctx, "sleep 1; true"); err == nil {
		t.Errorf("Didn't get error for timeout")
	}
}

func TestStart(t *testing.T) {
	td := newTestData(t)
	defer td.close()

	for _, tc := range []struct {
		cmd                  string // command to run
		im                   InputMode
		om                   OutputMode
		stdin                string // stdin to send
		expStdout, expStderr string // expected stdout and stderr
	}{
		{"true", CloseStdin, StdoutAndStderr, "", "", ""},
		{"echo foo", CloseStdin, StdoutAndStderr, "", "foo\n", ""},
		{"echo foo", CloseStdin, StdoutOnly, "", "foo\n", ""},
		{"echo foo", CloseStdin, StderrOnly, "", "", ""},
		{"echo foo", CloseStdin, NoOutput, "", "", ""},
		{"echo foo >&2", CloseStdin, StdoutAndStderr, "", "", "foo\n"},
		{"echo foo >&2", CloseStdin, StdoutOnly, "", "", ""},
		{"echo foo >&2", CloseStdin, StderrOnly, "", "", "foo\n"},
		{"echo foo >&2", CloseStdin, NoOutput, "", "", ""},
		{"echo out; echo err >&2", CloseStdin, StdoutAndStderr, "", "out\n", "err\n"},
		{"cat -", OpenStdin, StdoutAndStderr, "blah\n", "blah\n", ""},
		{"cat - >&2", OpenStdin, StdoutAndStderr, "blah\n", "", "blah\n"},
	} {
		ch, err := td.hst.Start(td.ctx, tc.cmd, tc.im, tc.om)
		if err != nil {
			t.Errorf("Failed to start %q: %v", tc.cmd, err)
			continue
		}

		if ch.Stdin() != nil {
			if _, err = ch.Stdin().Write([]byte(tc.stdin)); err != nil {
				t.Errorf("Failed to write %q to stdin for %q: %v", tc.stdin, tc.cmd, err)
			}
			if err = ch.Stdin().Close(); err != nil {
				t.Errorf("Failed closing stdin for %q: %v", tc.cmd, err)
			}
		}

		var stdout, stderr string
		if ch.Stdout() != nil {
			if b, err := ioutil.ReadAll(ch.Stdout()); err != nil {
				t.Errorf("Failed reading stdout for %q: %v", tc.cmd, err)
			} else {
				stdout = string(b)
			}
		}
		if stdout != tc.expStdout {
			t.Errorf("%q produced stdout %q; want %q", tc.cmd, stdout, tc.expStdout)
		}

		if ch.Stderr() != nil {
			if b, err := ioutil.ReadAll(ch.Stderr()); err != nil {
				t.Errorf("Failed reading stderr for %q: %v", tc.cmd, err)
			} else {
				stderr = string(b)
			}
		}
		if stderr != tc.expStderr {
			t.Errorf("%q produced stderr %q; want %q", tc.cmd, stderr, tc.expStderr)
		}

		if err := ch.Wait(td.ctx); err != nil {
			t.Errorf("Got error waiting for %q: %v", tc.cmd, err)
		}
		if err := ch.Close(td.ctx); err != nil {
			t.Errorf("Got error closing %q: %v", tc.cmd, err)
		}
	}
}

func TestRunTimeout(t *testing.T) {
	td := newTestData(t)
	defer td.close()

	td.srv.ExecDelays(0, 30*time.Second)
	ctx, cancel := context.WithTimeout(td.ctx, 10*time.Millisecond)
	defer cancel()
	if _, err := td.hst.Run(ctx, "true"); err == nil {
		t.Errorf("Run() with expired context didn't return error")
	}
}

func TestStartSessionTimeout(t *testing.T) {
	td := newTestData(t)
	defer td.close()

	td.srv.SessionDelay(30 * time.Second)
	ctx, cancel := context.WithTimeout(td.ctx, 10*time.Millisecond)
	defer cancel()
	if _, err := td.hst.Start(ctx, "true", CloseStdin, NoOutput); err == nil {
		t.Errorf("Start() with expired context didn't return error")
	}
}

func TestStartExecTimeout(t *testing.T) {
	td := newTestData(t)
	defer td.close()

	td.srv.ExecDelays(30*time.Second, 0)
	ctx, cancel := context.WithTimeout(td.ctx, 10*time.Millisecond)
	defer cancel()
	if _, err := td.hst.Start(ctx, "true", CloseStdin, NoOutput); err == nil {
		t.Errorf("Start() with expired context didn't return error")
	}
}

func TestWaitAndCloseTimeout(t *testing.T) {
	td := newTestData(t)
	defer td.close()

	h, err := td.hst.Start(td.ctx, "sleep 30", CloseStdin, NoOutput)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(td.ctx, time.Millisecond)
	defer cancel()
	if err = h.Wait(ctx); err == nil {
		t.Errorf("Wait() with expired context didn't return error")
	}
	if err = h.Close(ctx); err == nil {
		t.Errorf("Close() with expired context didn't return error")
	}
}

func TestPing(t *testing.T) {
	td := newTestData(t)
	defer td.close()

	if err := td.hst.Ping(td.ctx, time.Second); err != nil {
		t.Errorf("Got error when pinging host: %v", err)
	}

	// Use a short timeout.
	td.srv.AnswerPings(false)
	if err := td.hst.Ping(td.ctx, 10*time.Millisecond); err == nil {
		t.Errorf("Didn't get expected error when pinging host")
	}

	// Now set the timeout on the context instead.
	ctx, cancel := context.WithTimeout(td.ctx, 10*time.Millisecond)
	defer cancel()
	if err := td.hst.Ping(ctx, 10*time.Second); err == nil {
		t.Errorf("Didn't get expected error when pinging host")
	}
}

func TestGetFileRegular(t *testing.T) {
	td := newTestData(t)
	defer td.close()

	files := map[string]string{"file": "foo"}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	srcFile := filepath.Join(srcDir, "file")
	dstFile := filepath.Join(tmpDir, "file.copy")
	if err := td.hst.GetFile(td.ctx, srcFile, dstFile); err != nil {
		t.Fatal(err)
	}
	if err := checkFile(dstFile, files["file"]); err != nil {
		t.Error(err)
	}

	// GetFile should overwrite local files.
	if err := td.hst.GetFile(td.ctx, srcFile, dstFile); err != nil {
		t.Error(err)
	}
}

func TestGetFileDir(t *testing.T) {
	td := newTestData(t)
	defer td.close()

	files := map[string]string{
		"myfile":     "some data",
		"mydir/file": "this is in a subdirectory",
	}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	// Copy the full source directory.
	dstDir := filepath.Join(tmpDir, "dst")
	if err := td.hst.GetFile(td.ctx, srcDir, dstDir); err != nil {
		t.Fatal(err)
	}
	if err := checkDir(dstDir, files); err != nil {
		t.Error(err)
	}

	// GetFile should overwrite local dirs.
	if err := td.hst.GetFile(td.ctx, srcDir, dstDir); err != nil {
		t.Error(err)
	}
}

func TestGetFileTimeout(t *testing.T) {
	td := newTestData(t)
	defer td.close()

	files := map[string]string{"file": "data"}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	td.srv.ExecDelays(0, 30*time.Second)

	srcFile := filepath.Join(srcDir, "file")
	dstFile := filepath.Join(tmpDir, "file")
	ctx, cancel := context.WithTimeout(td.ctx, 10*time.Millisecond)
	defer cancel()
	if err := td.hst.GetFile(ctx, srcFile, dstFile); err == nil {
		t.Errorf("GetFile() with expired context didn't return error")
	}
}

func TestPutTree(t *testing.T) {
	td := newTestData(t)
	defer td.close()

	files := map[string]string{
		"file1":             "first file",
		"dir/file2":         "second file",
		"dir2/subdir/file3": "third file",
		"file4":             "new content",
	}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	// Create a directory and some files within the destination dir that should be overwritten.
	dstDir := filepath.Join(tmpDir, "dst")
	// TODO(derat): Create a file named "dir2" to check that files can be overwritten by
	// by directories. tar --recursive-unlink seems to be confused by this and fail with
	// a "Cannot unlink: Not a directory" error after trying to unlink a nonexistent
	// dir2/subdir/file3 file in the destination dir.
	if err := testutil.WriteFiles(dstDir, map[string]string{
		"file1/foo": "this file's dir should be overwritten by a file",
		"file4":     "old content",
		"existing":  "this file should be preserved",
	}); err != nil {
		t.Fatal(err)
	}

	// Request that three of the four files be copied.
	if n, err := td.hst.PutTree(td.ctx, srcDir, dstDir,
		[]string{"file1", "dir2/subdir/file3", "file4"}); err != nil {
		t.Fatal(err)
	} else if n <= 0 {
		t.Errorf("Copied non-positive %v bytes", n)
	}

	// The skipped file shouldn't be present, but the existing file should still be there.
	delete(files, "dir/file2")
	files["existing"] = "this file should be preserved"
	if err := checkDir(dstDir, files); err != nil {
		t.Error(err)
	}
}

func TestPutTreeRename(t *testing.T) {
	td := newTestData(t)
	defer td.close()

	const (
		weirdSrcName = "weird[a], *.b" // various regexp chars plus comma delimiter
		weirdDstName = "\\1\\a,&"      // sed replacement chars plus comma delimiter
	)
	files := map[string]string{
		"file1":             "file1 new",
		"dir/file2":         "file2 new",
		"dir2/subdir/file3": "file3 new",
		"file4":             "file4 new",
		weirdSrcName:        "file5 new",
		"file6":             "file6 new",
	}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	dstDir := filepath.Join(tmpDir, "dst")
	if err := testutil.WriteFiles(dstDir, map[string]string{
		"file1":             "file1 old",
		"dir/file2":         "file2 old",
		"dir2/subdir/file3": "file3 old",
		"file4":             "file4 old",
	}); err != nil {
		t.Fatal(err)
	}

	if n, err := td.hst.PutTreeRename(td.ctx, srcDir, dstDir, map[string]string{
		"file1":             "newfile1",           // rename to preserve orig file
		"dir/file2":         "dir/file2",          // overwrite orig file
		"dir2/subdir/file3": "dir2/subdir2/file3", // rename subdir
		weirdSrcName:        "file5",              // check that regexp chars are escaped
		"file6":             weirdDstName,         // check that replacement chars are also escaped
	}); err != nil {
		t.Fatal(err)
	} else if n <= 0 {
		t.Errorf("Copied non-positive %v bytes", n)
	}

	if err := checkDir(dstDir, map[string]string{
		"file1":              "file1 old",
		"newfile1":           "file1 new",
		"dir/file2":          "file2 new",
		"dir2/subdir/file3":  "file3 old",
		"dir2/subdir2/file3": "file3 new",
		"file4":              "file4 old",
		"file5":              "file5 new",
		weirdDstName:         "file6 new",
	}); err != nil {
		t.Error(err)
	}
}

func TestPutTreeUnchanged(t *testing.T) {
	td := newTestData(t)
	defer td.close()

	files := map[string]string{
		"file1":     "file1",
		"dir/file2": "file2",
	}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	dstDir := filepath.Join(tmpDir, "dst")
	if err := testutil.WriteFiles(dstDir, files); err != nil {
		t.Fatal(err)
	}

	// No bytes should be sent since the source and dest dirs are exactly the same.
	if n, err := td.hst.PutTree(td.ctx, srcDir, dstDir, []string{"file1", "dir/file2"}); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Errorf("PutTree() copied %v bytes; want 0", n)
	}
}

func TestPutTreeRenameUnchanged(t *testing.T) {
	td := newTestData(t)
	defer td.close()

	files := map[string]string{
		"src1":     "1",
		"dir/src2": "2",
	}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	dstDir := filepath.Join(tmpDir, "dst")
	if err := testutil.WriteFiles(dstDir, map[string]string{
		"dst1":     "1",
		"dir/dst2": "2",
	}); err != nil {
		t.Fatal(err)
	}

	// No bytes should be sent since the dest dir already contains the renamed source files.
	if n, err := td.hst.PutTreeRename(td.ctx, srcDir, dstDir, map[string]string{
		"src1":     "dst1",
		"dir/src2": "dir/dst2",
	}); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Errorf("PutTreeRename() copied %v bytes; want 0", n)
	}
}

func TestPutTreeTimeout(t *testing.T) {
	td := newTestData(t)
	defer td.close()

	files := map[string]string{"file": "data"}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	td.srv.ExecDelays(0, 30*time.Second)

	dstDir := filepath.Join(tmpDir, "dst")
	ctx, cancel := context.WithTimeout(td.ctx, 10*time.Millisecond)
	defer cancel()
	if _, err := td.hst.PutTree(ctx, srcDir, dstDir, []string{"file"}); err == nil {
		t.Errorf("PutTree() with expired context didn't return error")
	}
}

func TestKeyDir(t *testing.T) {
	srv, err := test.NewSSHServer(&userKey.PublicKey, hostKey)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	keyFile, err := test.WriteKey(userKey)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(keyFile)

	td := testutil.TempDir(t, "ssh_test.")
	defer os.RemoveAll(td)
	if err = os.Symlink(keyFile, filepath.Join(td, "testing_rsa")); err != nil {
		t.Fatal(err)
	}

	opt := SSHOptions{KeyDir: td}
	if err = ParseSSHTarget(srv.Addr().String(), &opt); err != nil {
		t.Fatal(err)
	}
	hst, err := NewSSH(context.Background(), &opt)
	if err != nil {
		t.Fatal(err)
	}
	hst.Close(context.Background())
}
