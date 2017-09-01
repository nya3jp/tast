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
)

func TestBuildTests(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "build_test.")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	cfg := &Config{
		TestWorkspace: tempDir,
		OutDir:        filepath.Join(tempDir, "out"),
	}
	if cfg.Arch, err = GetLocalArch(); err != nil {
		t.Fatal("Failed to get local arch: ", err)
	}

	srcDir := filepath.Join(tempDir, "src", "foo", "cmd")
	if err = os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	const exp = "success!"
	code := fmt.Sprintf("package main\nfunc main() { print(\"%s\") }", exp)
	if err := ioutil.WriteFile(filepath.Join(srcDir, "main.go"), []byte(code), 0644); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(tempDir, "foo")
	if out, err := BuildTests(context.Background(), cfg, "foo/cmd", bin); err != nil {
		t.Fatalf("Failed to build: %v: %s", err, string(out))
	}

	if out, err := exec.Command(bin).CombinedOutput(); err != nil {
		t.Errorf("Failed to run %s: %v", bin, err)
	} else if string(out) != exp {
		t.Errorf("%s printed %q; want %q", bin, string(out), exp)
	}
}
