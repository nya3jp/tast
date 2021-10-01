// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package breakpad

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"chromiumos/tast/cmd/tast/internal/symbolize/fakecmd"
	"chromiumos/tast/testutil"
)

// testSymbolFileInfo contains information about a symbol file written for testing.
type testSymbolFileInfo struct {
	name string // base name of file, e.g. "libfoo.so"
	id   string // see ModuleInfo.ID
	data string // symbol file contents
}

// getPath returns the symbol file's path within dir (which may be empty to omit a leading dir).
func (f *testSymbolFileInfo) getPath(dir string) string {
	return GetSymbolFilePath(dir, f.name, f.id)
}

// write writes the symbol file to the expected location within dir.
func (f *testSymbolFileInfo) write(dir string) error {
	p := f.getPath(dir)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	return ioutil.WriteFile(p, []byte(f.data), 0644)
}

func TestDownloadSymbols(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	files := []testSymbolFileInfo{
		{"mylib.so", "F0F0", "mylib symbols"},
		{"mybin", "1234", "mybin symbols"},
		{"otherlib.so", "ABCD", "otherlib symbols"},
	}

	// Create an archive containing the symbol files under relative paths starting
	// with imageArchiveTarPrefix.
	ad := filepath.Join(td, "archive")
	for _, f := range files {
		if err := f.write(filepath.Join(ad, imageArchiveTarPrefix)); err != nil {
			t.Fatal(err)
		}
	}
	af := filepath.Join(td, "symbols.tar.xz")
	cmd := exec.Command("tar", "--create", "--xz", "--file", af, "--directory", ad, imageArchiveTarPrefix)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Extract the first two symbol files.
	od := filepath.Join(td, "out")
	wanted := SymbolFileMap{files[0].name: files[0].id, files[1].name: files[1].id}
	if created, err := DownloadSymbols(af, od, wanted); err != nil {
		t.Fatalf("DownloadSymbols(%q, %q, %v) failed: %v", af, od, wanted, err)
	} else if created != 2 {
		t.Errorf("DownloadSymbols(%q, %q, %v) reported %v file(s); want 2", af, od, wanted, created)
	}

	// Verify that the expected files were written.
	act, err := testutil.ReadFiles(od)
	if err != nil {
		t.Fatal(err)
	}
	exp := map[string]string{
		files[0].getPath(""): files[0].data,
		files[1].getPath(""): files[1].data,
	}
	if !reflect.DeepEqual(act, exp) {
		t.Errorf("DownloadSymbols(%q, %q, %v) wrote %v; want %v", af, od, wanted, act, exp)
	}
}

func TestDownloadLacrosSymbols(t *testing.T) {
	cleanUp, err := fakecmd.Install("../fakecmd/scripts/gsutil")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanUp()

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	if err := exec.Command("cp", "testdata/lacros_debug.zip", td).Run(); err != nil {
		t.Errorf("Cannot copy lacros_debug.zip to the test directory %s, error: %v", td, err)
	}

	if err := DownloadLacrosSymbols("gs://ignored", td); err != nil {
		t.Errorf("DownloadLacrosSymbol failed: %v", err)
	}

	symbolFile := filepath.Join(td, "chrome/BE886B771A8C5C0AE60EBE6406B6E48F0/chrome.sym")
	if _, err := os.Stat(symbolFile); err != nil {
		t.Errorf("Symbol file not generated: %v", err)
	}
}
