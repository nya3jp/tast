// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package symbolize

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"chromiumos/tast/cmd/tast/internal/symbolize/breakpad"
	"chromiumos/tast/testutil"
)

func TestCreateSymbolFiles(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	// Libraries in testdata. IDs were obtained by running dump_syms.
	symMap := breakpad.SymbolFileMap{
		"/lib64/libpcprofile.so": "573F9EC9D1E952ED53CCD704E5BB6CC40",
		"/lib64/libutil-2.23.so": "0A356B7CFBCF5319947461F231A7D17C0",
	}
	cfg := Config{
		SymbolDir: filepath.Join(td, "symbols"),
		BuildRoot: filepath.Join(td, "build"),
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// Create a fake build root and symlink each debug library to the expected location in it.
	for lib := range symMap {
		dataPath := filepath.Join(cwd, "testdata", filepath.Base(lib)+".debug")
		debugPath := breakpad.GetDebugBinaryPath(cfg.BuildRoot, lib)
		if err := os.MkdirAll(filepath.Dir(debugPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(dataPath, debugPath); err != nil {
			t.Fatal(err)
		}
	}

	if created := createSymbolFiles(context.Background(), &cfg, symMap); created != len(symMap) {
		t.Errorf("createSymbolFiles(%v, %v) = %v; want %v", cfg, symMap, created, len(symMap))
	}
	for lib, id := range symMap {
		p := breakpad.GetSymbolFilePath(cfg.SymbolDir, filepath.Base(lib), id)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Errorf("createSymbolFiles(%v, %v) didn't create %v", cfg, symMap, p)
		}
	}
}
