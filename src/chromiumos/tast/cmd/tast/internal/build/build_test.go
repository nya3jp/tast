// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package build_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"testing"

	"chromiumos/tast/cmd/tast/internal/build"
	"chromiumos/tast/testutil"
)

func TestBuild(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	// In order to test that the supplied common and system GOPATHs are used, build a main
	// package that prints constants exported by a common and system package.
	const (
		testDir   = "test"
		commonDir = "common"
		sysDir    = "sys"

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

	if err := testutil.WriteFiles(td, map[string]string{
		filepath.Join(commonDir, "src", commonPkgName, "lib.go"): commonPkgCode,
		filepath.Join(sysDir, "src", sysPkgName, "lib.go"):       sysPkgCode,
		filepath.Join(testDir, "src", mainPkgName, "main.go"):    mainCode,
	}); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(td, "out")

	cfg := &build.Config{}
	tgt := &build.Target{
		Pkg:  mainPkgName,
		Arch: build.ArchHost,
		Workspaces: []string{
			filepath.Join(td, testDir),
			filepath.Join(td, commonDir),
			filepath.Join(td, sysDir),
		},
		Out: filepath.Join(outDir, path.Base(mainPkgName)),
	}

	if err := build.Build(context.Background(), cfg, []*build.Target{tgt}); err != nil {
		t.Fatal("Failed to build: ", err)
	}

	exp := commonConstVal + sysConstVal
	bin := filepath.Join(outDir, path.Base(mainPkgName))
	if out, err := exec.Command(bin).CombinedOutput(); err != nil {
		t.Errorf("Failed to run %s: %v", bin, err)
	} else if string(out) != exp {
		t.Errorf("%s printed %q; want %q", bin, string(out), exp)
	}
}

func TestBuildMulti(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	const (
		code = `package main

func main() {
}`

		wsDir = "ws"
		pkg1  = "pkg1"
		pkg2  = "pkg2"
	)

	if err := testutil.WriteFiles(td, map[string]string{
		filepath.Join(wsDir, "src", pkg1, "main.go"): code,
		filepath.Join(wsDir, "src", pkg2, "main.go"): code,
	}); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(td, "out")

	cfg := &build.Config{}
	tgts := []*build.Target{
		{
			Pkg:        pkg1,
			Arch:       build.ArchHost,
			Workspaces: []string{filepath.Join(td, wsDir)},
			Out:        filepath.Join(outDir, pkg1),
		},
		{
			Pkg:        pkg2,
			Arch:       build.ArchHost,
			Workspaces: []string{filepath.Join(td, wsDir)},
			Out:        filepath.Join(outDir, pkg2),
		},
	}

	if err := build.Build(context.Background(), cfg, tgts); err != nil {
		t.Fatal("Failed to build: ", err)
	}

	for _, pkg := range []string{pkg1, pkg2} {
		if err := exec.Command(filepath.Join(outDir, pkg)).Run(); err != nil {
			t.Errorf("Failed to run %s: %v", pkg, err)
		}
	}
}

func TestBuildBadWorkspace(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	if err := testutil.WriteFiles(td, map[string]string{
		"ws1/src/good/main.go": "package main\nfunc main() {}\n",
		"ws2/bad/foo.go":       "package bad\n",
	}); err != nil {
		t.Fatal(err)
	}

	cfg := &build.Config{}
	tgt := &build.Target{
		Pkg:        "good",
		Arch:       build.ArchHost,
		Workspaces: []string{filepath.Join(td, "ws1")},
		Out:        filepath.Join(td, "out/good"),
	}

	if err := build.Build(context.Background(), cfg, []*build.Target{tgt}); err != nil {
		t.Fatal("Failed to build: ", err)
	}

	tgt.Workspaces = append(tgt.Workspaces, filepath.Join(td, "ws2"))

	if err := build.Build(context.Background(), cfg, []*build.Target{tgt}); err == nil {
		t.Fatal("Building with invalid workspace unexpectedly succeeded")
	}
}
