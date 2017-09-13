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
		pkgName, pkgName, constName)

	if err := testutil.WriteFiles(tempDir, map[string]string{
		filepath.Join("src", pkgName, "lib.go"): pkgCode,
		"src/foo/cmd/main.go":                   mainCode,
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
