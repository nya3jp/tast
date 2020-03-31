// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package host

import (
	"context"
	"crypto/rsa"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"chromiumos/tast/internal/sshtest"
	"chromiumos/tast/testutil"
)

var userKey, hostKey *rsa.PrivateKey

func init() {
	userKey, hostKey = sshtest.MustGenerateKeys()
}

// connectToServer establishes a connection to srv using key.
// base is used as a base set of options.
func connectToServer(ctx context.Context, srv *sshtest.SSHServer, key *rsa.PrivateKey, base *SSHOptions) (*SSH, error) {
	keyFile, err := sshtest.WriteKey(key)
	if err != nil {
		return nil, err
	}
	defer os.Remove(keyFile)

	o := *base
	o.KeyFile = keyFile
	if err = ParseSSHTarget(srv.Addr().String(), &o); err != nil {
		return nil, err
	}
	s, err := NewSSH(ctx, &o)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// timeoutType describes different types of timeouts that can be simulated during SSH "exec" requests.
type timeoutType int

const (
	// noTimeout indicates that testData.ctx shouldn't be canceled.
	noTimeout timeoutType = iota
	// startTimeout indicates that testData.ctx should be canceled before the command starts.
	startTimeout
	// endTimeout indicates that testData.ctx should be canceled after the command runs but before its status is returned.
	endTimeout
)

// testData wraps data common to all tests.
type testData struct {
	srv *sshtest.SSHServer
	hst *SSH

	ctx    context.Context // used for performing operations using hst
	cancel func()          // cancels ctx to simulate a timeout

	nextCmd     string      // next command to be executed by client
	execTimeout timeoutType // how "exec" requests should time out
}

func newTestData(t *testing.T) *testData {
	td := &testData{}
	td.ctx, td.cancel = context.WithCancel(context.Background())

	var err error
	if td.srv, err = sshtest.NewSSHServer(&userKey.PublicKey, hostKey, td.handleExec); err != nil {
		t.Fatal(err)
	}

	if td.hst, err = connectToServer(td.ctx, td.srv, userKey, &SSHOptions{}); err != nil {
		td.srv.Close()
		t.Fatal(err)
	}
	td.hst.AnnounceCmd = func(cmd string) { td.nextCmd = cmd }

	// Automatically abort the test if it takes too long time.
	go func() {
		const timeout = 10 * time.Second
		select {
		case <-td.ctx.Done():
			return
		case <-time.After(timeout):
		}
		t.Errorf("Test blocked for %v", timeout)
		td.cancel()
	}()

	return td
}

func (td *testData) close() {
	td.srv.Close()
	td.hst.Close(td.ctx)
	td.cancel()
}

// handleExec handles an SSH "exec" request sent to td.srv by executing the requested command.
// The command must already be present in td.nextCmd.
func (td *testData) handleExec(req *sshtest.ExecReq) {
	if req.Cmd != td.nextCmd {
		log.Printf("Unexpected command %q (want %q)", req.Cmd, td.nextCmd)
		req.Start(false)
		return
	}

	// PutFiles sends multiple "exec" requests.
	// Ignore its initial "sha1sum" so we can hang during the tar command instead.
	ignoreTimeout := strings.HasPrefix(req.Cmd, "sha1sum ")

	// If a timeout was requested, cancel the context and then sleep for an arbitrary-but-long
	// amount of time to make sure that the client sees the expired context before the command
	// actually runs.
	if td.execTimeout == startTimeout && !ignoreTimeout {
		td.cancel()
		time.Sleep(time.Minute)
	}
	req.Start(true)

	var status int
	switch req.Cmd {
	case shellCmd("", []string{"long_sleep"}):
		time.Sleep(time.Hour)
	default:
		status = req.RunRealCmd()
	}

	if td.execTimeout == endTimeout && !ignoreTimeout {
		td.cancel()
		time.Sleep(time.Minute)
	}
	req.End(status)
}

// initFileTest creates a temporary directory with a subdirectory containing files.
// The temp dir's and subdir's paths are returned.
func initFileTest(t *testing.T, files map[string]string) (tmpDir, srcDir string) {
	tmpDir = testutil.TempDir(t)
	srcDir = filepath.Join(tmpDir, "src")
	if err := os.Mkdir(srcDir, 0755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}
	if err := testutil.WriteFiles(srcDir, files); err != nil {
		os.RemoveAll(tmpDir)
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
	if hst, err := connectToServer(ctx, srv, userKey, &SSHOptions{ConnectRetries: 1}); err == nil {
		t.Error("Unexpectedly able to connect to server with inadequate retries")
		hst.Close(ctx)
	}

	// With two retries (i.e. three attempts), the connection should be successfully established.
	srv.RejectConns(2)
	if hst, err := connectToServer(ctx, srv, userKey, &SSHOptions{ConnectRetries: 2}); err != nil {
		t.Error("Failed connecting to server despite adequate retries: ", err)
	} else {
		hst.Close(ctx)
	}
}

func TestPing(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	td.srv.AnswerPings(true)
	if err := td.hst.Ping(td.ctx, time.Minute); err != nil {
		t.Errorf("Got error when pinging host: %v", err)
	}

	td.srv.AnswerPings(false)
	if err := td.hst.Ping(td.ctx, time.Millisecond); err == nil {
		t.Errorf("Didn't get expected error when pinging host with short timeout")
	}

	// Cancel the context to simulate it having expired.
	td.cancel()
	if err := td.hst.Ping(td.ctx, time.Minute); err == nil {
		t.Errorf("Didn't get expected error when pinging host with expired context")
	}
}

func TestGetFileRegular(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	files := map[string]string{"file": "foo"}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	srcFile := filepath.Join(srcDir, "file")
	dstFile := filepath.Join(tmpDir, "file.copy")
	if err := td.hst.DeprecatedGetFile(td.ctx, srcFile, dstFile); err != nil {
		t.Fatal(err)
	}
	if err := checkFile(dstFile, files["file"]); err != nil {
		t.Error(err)
	}

	// GetFile should overwrite local files.
	if err := td.hst.DeprecatedGetFile(td.ctx, srcFile, dstFile); err != nil {
		t.Error(err)
	}
}

func TestGetFileDir(t *testing.T) {
	t.Parallel()
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
	if err := td.hst.DeprecatedGetFile(td.ctx, srcDir, dstDir); err != nil {
		t.Fatal(err)
	}
	if err := checkDir(dstDir, files); err != nil {
		t.Error(err)
	}

	// GetFile should overwrite local dirs.
	if err := td.hst.DeprecatedGetFile(td.ctx, srcDir, dstDir); err != nil {
		t.Error(err)
	}
}

func TestGetFileTimeout(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	files := map[string]string{"file": "data"}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	srcFile := filepath.Join(srcDir, "file")
	dstFile := filepath.Join(tmpDir, "file")
	td.execTimeout = startTimeout
	if err := td.hst.DeprecatedGetFile(td.ctx, srcFile, dstFile); err == nil {
		t.Errorf("GetFile() with expired context didn't return error")
	}
}

func TestPutFiles(t *testing.T) {
	t.Parallel()
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

	if n, err := td.hst.DeprecatedPutFiles(td.ctx, map[string]string{
		filepath.Join(srcDir, "file1"):             filepath.Join(dstDir, "newfile1"),           // rename to preserve orig file
		filepath.Join(srcDir, "dir/file2"):         filepath.Join(dstDir, "dir/file2"),          // overwrite orig file
		filepath.Join(srcDir, "dir2/subdir/file3"): filepath.Join(dstDir, "dir2/subdir2/file3"), // rename subdir
		filepath.Join(srcDir, weirdSrcName):        filepath.Join(dstDir, "file5"),              // check that regexp chars are escaped
		filepath.Join(srcDir, "file6"):             filepath.Join(dstDir, weirdDstName),         // check that replacement chars are also escaped
	}, DeprecatedPreserveSymlinks); err != nil {
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

func TestPutFilesUnchanged(t *testing.T) {
	t.Parallel()
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
	if n, err := td.hst.DeprecatedPutFiles(td.ctx, map[string]string{
		filepath.Join(srcDir, "src1"):     filepath.Join(dstDir, "dst1"),
		filepath.Join(srcDir, "dir/src2"): filepath.Join(dstDir, "dir/dst2"),
	}, DeprecatedPreserveSymlinks); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Errorf("PutFiles() copied %v bytes; want 0", n)
	}
}

func TestPutFilesTimeout(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	files := map[string]string{"file": "data"}
	tmpDir, srcDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)
	dstDir := filepath.Join(tmpDir, "dst")
	td.execTimeout = endTimeout
	if _, err := td.hst.DeprecatedPutFiles(td.ctx, map[string]string{
		filepath.Join(srcDir, "file"): filepath.Join(dstDir, "file"),
	}, DeprecatedPreserveSymlinks); err == nil {
		t.Errorf("PutFiles() with expired context didn't return error")
	}
}

func TestPutFilesSymlinks(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	tmpDir, srcDir := initFileTest(t, nil)
	defer os.RemoveAll(tmpDir)

	// Create a symlink pointing to an executable file outside of the source dir.
	const (
		data = "executable file"
		link = "symlink"
		mode = 0755
	)
	targetPath := filepath.Join(tmpDir, "exe")
	if err := ioutil.WriteFile(targetPath, []byte(data), mode); err != nil {
		t.Fatalf("Failed to write %v: %v", targetPath, err)
	}
	if err := os.Symlink(targetPath, filepath.Join(srcDir, link)); err != nil {
		t.Fatalf("Failed to create symlink to %v: %v", targetPath, err)
	}

	// PreserveSymlinks should copy symlinks directly.
	dstDir := filepath.Join(tmpDir, "dst_preserve")
	if _, err := td.hst.DeprecatedPutFiles(td.ctx, map[string]string{
		filepath.Join(srcDir, link): filepath.Join(dstDir, link),
	}, DeprecatedPreserveSymlinks); err != nil {
		t.Error("PutFiles failed with PreserveSymlinks: ", err)
	} else {
		dstFile := filepath.Join(dstDir, link)
		if fi, err := os.Lstat(dstFile); err != nil {
			t.Errorf("Failed to lstat %v: %v", dstFile, err)
		} else if fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%v is not a symlink; got mode %v", dstFile, fi.Mode())
		} else if target, err := os.Readlink(dstFile); err != nil {
			t.Errorf("Failed to read %v: %v", dstFile, err)
		} else if target != targetPath {
			t.Errorf("%v has target %v; want %v", dstFile, target, targetPath)
		}
	}

	// DereferenceSymlinks should copy symlinks' targets while preserving the original mode.
	dstDir = filepath.Join(tmpDir, "dst_deref")
	if _, err := td.hst.DeprecatedPutFiles(td.ctx, map[string]string{
		filepath.Join(srcDir, link): filepath.Join(dstDir, link),
	}, DeprecatedDereferenceSymlinks); err != nil {
		t.Error("PutFiles failed with DereferenceSymlinks: ", err)
	} else {
		dstFile := filepath.Join(dstDir, link)
		if fi, err := os.Lstat(dstFile); err != nil {
			t.Errorf("Failed to lstat %v: %v", dstFile, err)
		} else if fi.Mode()&os.ModeType != 0 {
			t.Errorf("%v is not a regular file; got mode %v", dstFile, fi.Mode())
		} else if perm := fi.Mode() & os.ModePerm; perm != mode {
			t.Errorf("%v has perms %#o; want %#o", dstFile, perm, mode)
		}
		if b, err := ioutil.ReadFile(dstFile); err != nil {
			t.Errorf("Failed to read %v: %v", dstFile, err)
		} else if string(b) != data {
			t.Errorf("%v contains %q; want %q", dstFile, b, data)
		}
	}
}

func TestDeleteTree(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	files := map[string]string{
		"file1":     "first file",
		"file2":     "second file",
		"dir/file3": "third file",
		"dir/file4": "fourth file",
	}
	tmpDir, baseDir := initFileTest(t, files)
	defer os.RemoveAll(tmpDir)

	if err := td.hst.DeprecatedDeleteTree(td.ctx, baseDir, []string{"file1", "dir", "file9"}); err != nil {
		t.Fatal(err)
	}

	expected := map[string]string{"file2": "second file"}
	if err := checkDir(baseDir, expected); err != nil {
		t.Error(err)
	}
}

func TestDeleteTreeOutside(t *testing.T) {
	t.Parallel()
	td := newTestData(t)
	defer td.close()

	tmpDir, baseDir := initFileTest(t, nil)
	defer os.RemoveAll(tmpDir)

	if err := td.hst.DeprecatedDeleteTree(td.ctx, baseDir, []string{"dir/../../outside"}); err == nil {
		t.Error("DeleteTree succeeded; should fail")
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
