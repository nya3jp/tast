// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package build

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"chromiumos/tast/common/testutil"
)

func TestBuildTests(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "build_test.")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	sysGopath := filepath.Join(tempDir, "gopath")
	cfg := &Config{
		TestWorkspace: tempDir,
		SysGopath:     sysGopath,
		OutDir:        filepath.Join(tempDir, "out"),
	}
	if cfg.Arch, err = GetLocalArch(); err != nil {
		t.Fatal("Failed to get local arch: ", err)
	}

	// In order to test that the supplied system GOPATH is used, build a main
	// package that prints a constant exported by a system package.
	const (
		pkgName   = "testpkg"  // system package's name (without chromiumos/tast prefix)
		constName = "Msg"      // name of const exported by system package
		constVal  = "success!" // value of const exported by system package
	)
	pkgCode := fmt.Sprintf("package %s\nconst %s = %q", pkgName, constName, constVal)
	mainCode := fmt.Sprintf("package main\nimport %q\nfunc main() { print(%s.%s) }",
		"chromiumos/tast/"+pkgName, pkgName, constName)

	if err := testutil.WriteFiles(tempDir, map[string]string{
		filepath.Join(baseTastSrcDir, pkgName, "lib.go"): pkgCode,
		"src/foo/cmd/main.go":                            mainCode,
	}); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(tempDir, "foo")
	if out, err := BuildTests(context.Background(), cfg, "foo/cmd", bin); err != nil {
		t.Fatalf("Failed to build: %v: %s", err, string(out))
	}

	if out, err := exec.Command(bin).CombinedOutput(); err != nil {
		t.Errorf("Failed to run %s: %v", bin, err)
	} else if string(out) != constVal {
		t.Errorf("%s printed %q; want %q", bin, string(out), constVal)
	}
}

func TestFreshestSysGopath(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "build_test.")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	for _, s := range []struct {
		dir, fn string // dir within temp dir and filename
		offset  int64  // offset from Unix epoch for file's mtime
	}{
		{"oldest", "a.go", 1},
		{"newer", "b.go", 3},
		{"newest", "old.go", 2},
		{"newest", "subdir/new.go", 4},
	} {
		p := filepath.Join(tempDir, s.dir, baseTastSrcDir, s.fn)
		if err = os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err = ioutil.WriteFile(p, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}

		// Change the file's mtime to the requested offset past the Unix epoch,
		// and then change its parent directories to the epoch itself.
		ft := time.Unix(s.offset, 0)
		if err = os.Chtimes(p, ft, ft); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Dir(p)
		for len(dir) > len(tempDir) {
			if err = os.Chtimes(dir, time.Unix(0, 0), time.Unix(0, 0)); err != nil {
				t.Fatal(err)
			}
			dir = filepath.Dir(dir)
		}
	}

	exp := filepath.Join(tempDir, "newest")
	pat := tempDir + "/*"
	if act, err := FreshestSysGopath(pat); err != nil {
		t.Errorf("FreshestSysGopath(%q) returned error: %v", pat, err)
	} else if act != exp {
		t.Errorf("FreshestSysGopath(%q) = %q; want %q", pat, act, exp)
	}
}
