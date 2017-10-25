// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package build

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"chromiumos/tast/common/testutil"
)

func TestBuildTests(t *testing.T) {
	tempDir := testutil.TempDir(t, "build_test.")
	defer os.RemoveAll(tempDir)

	const (
		testDir   = "test"
		commonDir = "common"
		sysDir    = "sys"
	)

	var err error
	cfg := &Config{
		TestWorkspace:   filepath.Join(tempDir, testDir),
		CommonWorkspace: filepath.Join(tempDir, commonDir),
		SysGopath:       filepath.Join(tempDir, sysDir),
		OutDir:          filepath.Join(tempDir, "out"),
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
	)
	commonPkgCode := fmt.Sprintf("package %s\nconst %s = %q", commonPkgName, commonConstName, commonConstVal)
	sysPkgCode := fmt.Sprintf("package %s\nconst %s = %q", sysPkgName, sysConstName, sysConstVal)
	mainCode := fmt.Sprintf("package main\nimport %q\nimport %q\nfunc main() { print(%s.%s+%s.%s) }",
		commonPkgName, sysPkgName, commonPkgName, commonConstName, sysPkgName, sysConstName)

	if err := testutil.WriteFiles(tempDir, map[string]string{
		filepath.Join(commonDir, "src", commonPkgName, "lib.go"): commonPkgCode,
		filepath.Join(sysDir, "src", sysPkgName, "lib.go"):       sysPkgCode,
		filepath.Join(testDir, "src/foo/cmd/main.go"):            mainCode,
	}); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(tempDir, "foo")
	if out, err := BuildTests(context.Background(), cfg, "foo/cmd", bin); err != nil {
		t.Fatalf("Failed to build: %v: %s", err, string(out))
	}

	exp := commonConstVal + sysConstVal
	if out, err := exec.Command(bin).CombinedOutput(); err != nil {
		t.Errorf("Failed to run %s: %v", bin, err)
	} else if string(out) != exp {
		t.Errorf("%s printed %q; want %q", bin, string(out), exp)
	}
}
