// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package linuxssh_test

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/logging/loggingtest"
	"go.chromium.org/tast/core/internal/sshtest"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testutil"
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

func TestLineCount(t *testing.T) {
	t.Parallel()
	td := sshtest.NewTestDataConn(t)
	defer td.Close()

	ctx := logging.AttachLogger(context.Background(), loggingtest.NewLogger(t, logging.LevelInfo)) // for debug

	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	filename := "foo.txt"
	files := map[string]string{filename: "line1\nline2\nline3\n"}
	testutil.WriteFiles(dir, files)

	want := &linuxssh.WordCountInfo{
		Lines: 3,
		Words: 3,
		Bytes: 18,
	}
	got, err := linuxssh.WordCount(ctx, td.Hst, filepath.Join(dir, filename))
	if err != nil {
		t.Errorf("Failed the get line count: %v", err)
	}
	if got.Lines != want.Lines {
		t.Errorf("Failed to get correct line count; got = %v, want %v", got.Lines, want.Lines)
	}
	if got.Words != want.Words {
		t.Errorf("Failed to get correct word count; got = %v, want %v", got.Words, want.Words)
	}
	if got.Bytes != want.Bytes {
		t.Errorf("Failed to get correct byte count; got = %v, want %v", got.Bytes, want.Bytes)
	}
}
