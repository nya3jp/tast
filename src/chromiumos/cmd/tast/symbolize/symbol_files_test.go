// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package symbolize

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"chromiumos/cmd/tast/logging"
	"chromiumos/cmd/tast/symbolize/breakpad"
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
	cfg := NewConfig(filepath.Join(td, "symbols"), filepath.Join(td, "build"),
		logging.NewSimple(&bytes.Buffer{}, 0, true))

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// Create a fake build root and symlink each debug library to the expected location in it.
	for lib := range symMap {
		dataPath := filepath.Join(cwd, "testdata", filepath.Base(lib)+".debug")
		debugPath := breakpad.GetDebugBinaryPath(cfg.buildRoot, lib)
		if err := os.MkdirAll(filepath.Dir(debugPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(dataPath, debugPath); err != nil {
			t.Fatal(err)
		}
	}

	wanted := make(breakpad.SymbolFileMap)
	for p, id := range symMap {
		wanted[p] = id
	}
	const (
		missingName = "/lib64/missing.so"
		missingID   = "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"
	)
	wanted[missingName] = missingID

	expMissing := breakpad.SymbolFileMap{missingName: missingID}
	if missing := createSymbolFiles(cfg, wanted); !reflect.DeepEqual(missing, expMissing) {
		t.Errorf("createSymbolFiles(%v, %v) = %v; want %v", cfg, wanted, missing, expMissing)
	}
	for lib, id := range symMap {
		p := breakpad.GetSymbolFilePath(cfg.symbolDir, filepath.Base(lib), id)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Errorf("createSymbolFiles(%v, %v) didn't create %v", cfg, symMap, p)
		}
	}
}
