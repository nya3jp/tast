// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package linuxssh_test

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/logging/loggingtest"
	"chromiumos/tast/internal/sshtest"
	"chromiumos/tast/ssh/linuxssh"
	"chromiumos/tast/testutil"
)

const strangeFileName = "$()\"'` "

func TestReadFile(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	ctx := logging.AttachLogger(context.Background(), loggingtest.NewLogger(t, logging.LevelInfo)) // for debug

	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	files := map[string]string{"foo.txt": "hello", strangeFileName: "ok"}
	testutil.WriteFiles(dir, files)

	for _, f := range []string{"foo.txt", strangeFileName} {
		b, err := linuxssh.ReadFile(ctx, td.Hst, filepath.Join(dir, f))
		if err != nil {
			t.Errorf("ReadFile(%q): %v", f, err)
		}
		if got, want := string(b), files[f]; got != want {
			t.Errorf("ReadFile(%q) = %q, want %q", f, got, want)
		}
	}

	if _, err := linuxssh.ReadFile(ctx, td.Hst, "not.exist"); err == nil {
		t.Errorf("ReadFile succeeds, want error")
	}
}

func TestWriteFile(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	ctx := logging.AttachLogger(context.Background(), loggingtest.NewLogger(t, logging.LevelInfo)) // for debug

	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	checkFile := func(path, content string, perm os.FileMode) {
		t.Helper()
		base := filepath.Base(path)
		if got, err := ioutil.ReadFile(path); err != nil {
			t.Fatal(err)
		} else if string(got) != content {
			t.Errorf("%v; content = %q, want %q", base, got, content)
		}
		if info, err := os.Stat(path); err != nil {
			t.Fatal(err)
		} else if info.Mode() != perm {
			t.Errorf("%v; File mode = %v, want %v", base, info.Mode(), perm)
		}
	}

	path1 := filepath.Join(dir, "existing.txt")
	if err := ioutil.WriteFile(path1, []byte("previous content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := linuxssh.WriteFile(ctx, td.Hst, path1, []byte("new content"), 0755); err != nil {
		t.Errorf("WriteFile: %v", err)
	}
	checkFile(path1, "new content", 0644)

	path2 := filepath.Join(dir, "new.txt")
	if err := linuxssh.WriteFile(ctx, td.Hst, path2, []byte("a"), 0755); err != nil {
		t.Errorf("WriteFile: %v", err)
	}
	checkFile(path2, "a", 0755)

	// Arbitrary filename and permissions should work.
	path3 := filepath.Join(dir, strangeFileName)
	if err := linuxssh.WriteFile(ctx, td.Hst, path3, []byte("b"), 0473); err != nil {
		t.Errorf("WriteFile: %v", err)
	}
	checkFile(path3, "b", 0473)
}
