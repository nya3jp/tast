// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fsutil

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"chromiumos/tast/testutil"
)

func TestCopyFile(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	src := filepath.Join(td, "src.txt")
	const data = "this is not the most interesting text ever written"
	if err := ioutil.WriteFile(src, []byte(data), 0461); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(td, "dst.txt")
	const dstMode = 0644
	dstUID := os.Getuid()

	if err := CopyFile(src, dst, CopyMode(dstMode), CopyOwner(dstUID, -1)); err != nil {
		t.Fatalf("CopyFile(%q, %q) failed: %v", src, dst, err)
	}

	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("Failed to stat %v: %v", dst, err)
	}
	if perm := fi.Mode() & os.ModePerm; perm != dstMode {
		t.Errorf("%v has mode 0%o; want 0%o", dst, perm, dstMode)
	}
	if uid := int(fi.Sys().(*syscall.Stat_t).Uid); uid != dstUID {
		t.Errorf("%v has UID %d; want %d", dst, uid, dstUID)
	}

	if b, err := ioutil.ReadFile(dst); err != nil {
		t.Errorf("Failed to read %v: %v", dst, err)
	} else if string(b) != data {
		t.Errorf("%v contains %q; want %q", dst, string(b), data)
	}
}

func TestMoveFile(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	const (
		data = "another boring file"
		mode = 0644
	)
	src := filepath.Join(td, "src.txt")
	if err := ioutil.WriteFile(src, []byte(data), mode); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(td, "dst.txt")
	if err := MoveFile(src, dst); err != nil {
		t.Fatalf("MoveFile(%q, %q) failed: %v", src, dst, err)
	}

	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("Failed to stat %v: %v", dst, err)
	} else if perm := fi.Mode() & os.ModePerm; perm != mode {
		t.Errorf("%v has mode 0%o; want 0%o", dst, perm, mode)
	}

	if b, err := ioutil.ReadFile(dst); err != nil {
		t.Errorf("Failed to read %v: %v", dst, err)
	} else if string(b) != data {
		t.Errorf("%v contains %q; want %q", dst, string(b), data)
	}

	if _, err := os.Stat(src); err == nil {
		t.Errorf("%s still exists", src)
	} else if !os.IsNotExist(err) {
		t.Errorf("Failed to stat %s: %v", src, err)
	}
}

func TestCopyFileOrMoveFileWithDir(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	src := filepath.Join(td, "dir")
	if err := os.Mkdir(src, 0755); err != nil {
		t.Fatal(err)
	}

	// Both functions should reject directories.
	if err := CopyFile(src, filepath.Join(td, "copyDst")); err == nil {
		t.Error("CopyFile unexpectedly succeeded for directory ", src)
	}
	if err := MoveFile(src, filepath.Join(td, "moveDst")); err == nil {
		t.Error("MoveFile unexpectedly succeeded for directory ", src)
	}
}
