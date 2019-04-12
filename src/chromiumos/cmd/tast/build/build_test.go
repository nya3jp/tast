// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package build

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"chromiumos/cmd/tast/logging"
	"chromiumos/tast/testutil"
)

func TestBuild(t *testing.T) {
	tempDir := testutil.TempDir(t)
	defer os.RemoveAll(tempDir)

	const (
		testDir   = "test"
		commonDir = "common"
		sysDir    = "sys"
	)

	var err error
	cfg := &Config{
		Logger: logging.NewSimple(&bytes.Buffer{}, 0, false),
		Workspaces: []string{
			filepath.Join(tempDir, testDir),
			filepath.Join(tempDir, commonDir),
			filepath.Join(tempDir, sysDir),
		},
	}
	if cfg.Arch, err = GetLocalArch(); err != nil {
		t.Fatal("Failed to get local arch: ", err)
	}

	// In order to test that the supplied common and system GOPATHs are used, build a main
	// package that prints constants exported by a common and system package.
	const (
		commonPkgName   = "commonpkg" // common package's name
		commonConstName = "Msg"       // name of const exported by common package
		commonConstVal  = "foo"       // value of const exported by common package

		sysPkgName   = "syspkg" // system package's name
		sysConstName = "Msg"    // name of const exported by system package
		sysConstVal  = "bar"    // value of const exported by system package

		mainPkgName = "foo" // main() package's name
	)
	commonPkgCode := fmt.Sprintf("package %s\nconst %s = %q", commonPkgName, commonConstName, commonConstVal)
	sysPkgCode := fmt.Sprintf("package %s\nconst %s = %q", sysPkgName, sysConstName, sysConstVal)
	mainCode := fmt.Sprintf("package main\nimport %q\nimport %q\nfunc main() { print(%s.%s+%s.%s) }",
		commonPkgName, sysPkgName, commonPkgName, commonConstName, sysPkgName, sysConstName)

	if err := testutil.WriteFiles(tempDir, map[string]string{
		filepath.Join(commonDir, "src", commonPkgName, "lib.go"): commonPkgCode,
		filepath.Join(sysDir, "src", sysPkgName, "lib.go"):       sysPkgCode,
		filepath.Join(testDir, "src", mainPkgName, "main.go"):    mainCode,
	}); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(tempDir, "out")
	if out, err := Build(context.Background(), cfg, mainPkgName, outDir, ""); err != nil {
		t.Fatalf("Failed to build: %v: %s", err, string(out))
	}

	exp := commonConstVal + sysConstVal
	bin := filepath.Join(outDir, filepath.Base(mainPkgName))
	if out, err := exec.Command(bin).CombinedOutput(); err != nil {
		t.Errorf("Failed to run %s: %v", bin, err)
	} else if string(out) != exp {
		t.Errorf("%s printed %q; want %q", bin, string(out), exp)
	}
}

func TestBuildBadWorkspace(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	var err error
	cfg := &Config{
		Logger:     logging.NewSimple(&bytes.Buffer{}, 0, false),
		Workspaces: []string{filepath.Join(td, "ws1")},
	}
	if cfg.Arch, err = GetLocalArch(); err != nil {
		t.Fatal("Failed to get local arch: ", err)
	}
	if err := testutil.WriteFiles(td, map[string]string{
		"ws1/src/good/main.go": "package main\nfunc main() {}\n",
		"ws2/bad/foo.go":       "package bad\n",
	}); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(td, "out")
	if out, err := Build(context.Background(), cfg, "good", outDir, ""); err != nil {
		t.Fatalf("Failed to build: %v: %s", err, string(out))
	}

	cfg.Workspaces = append(cfg.Workspaces, filepath.Join(td, "ws2"))
	if _, err := Build(context.Background(), cfg, "good", outDir, ""); err == nil {
		t.Fatal("Building with invalid workspace unexpectedly succeeded")
	}
}
