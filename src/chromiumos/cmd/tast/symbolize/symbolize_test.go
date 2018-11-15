// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package symbolize

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"chromiumos/cmd/tast/logging"
	"chromiumos/cmd/tast/symbolize/breakpad"
	"chromiumos/tast/testutil"
)

// testData contains data used for testing symbolize.SymbolizeCrash.
type testData struct {
	tempDir string
	cfg     *Config

	relInfo releaseInfo // returned by cfg.getMinidumpReleaseInfo

	// Number of times that functions were called.
	downloadCalls, createCalls, walkCalls int

	// Set by tests to control which files can be produced by downloadSymbols and createSymbolFiles.
	downloadable, creatable breakpad.SymbolFileMap

	// Set by tests to simulate errors.
	downloadErr, createErr error

	// Files that have been produced by downloadSymbols and createSymbolFiles. Used by walkMinidump.
	produced breakpad.SymbolFileMap

	// Map from minidump paths passed to walkMinidump to required symbol files.
	dumps map[string]breakpad.SymbolFileMap
}

func newTestData(t *testing.T) *testData {
	td := &testData{
		tempDir:  testutil.TempDir(t),
		produced: make(breakpad.SymbolFileMap),
		relInfo: releaseInfo{
			board:       "cave",                       // arbitrary
			builderPath: "cave-release/R65-10286.0.0", // arbitrary
		},
	}
	toClose := td
	defer func() {
		if toClose != nil {
			toClose.close()
		}
	}()

	symbolDir := filepath.Join(td.tempDir, "symbols")
	if err := os.Mkdir(symbolDir, 0755); err != nil {
		t.Fatal(err)
	}
	buildDir := filepath.Join(td.tempDir, "build")
	if err := os.Mkdir(buildDir, 0755); err != nil {
		t.Fatal(err)
	}
	td.cfg = NewConfig(symbolDir, buildDir, logging.NewSimple(&bytes.Buffer{}, 0, false))
	td.cfg.getMinidumpPath = func(cfg *Config, path string) (string, error) { return path, nil }
	td.cfg.getMinidumpReleaseInfo = func(path string) (*releaseInfo, error) { return &td.relInfo, nil }
	td.cfg.walkMinidump = td.walkMinidump
	td.cfg.downloadSymbols = td.downloadSymbols
	td.cfg.createSymbolFiles = td.createSymbolFiles

	toClose = nil
	return td
}

func (td *testData) close() {
	os.RemoveAll(td.tempDir)
}

// resetStats clears call counts.
func (td *testData) resetStats() {
	td.downloadCalls, td.createCalls, td.walkCalls = 0, 0, 0
}

// walkMinidump implements Config.walkMinidump.
// It checks whether the files registered in td.dumps[path] are present in td.produced.
// Files that are present are written to w per symbolFilesString.
// files that aren't present are returned via missing.
func (td *testData) walkMinidump(path, symDir string, w io.Writer) (missing breakpad.SymbolFileMap, err error) {
	td.walkCalls++
	if symDir != td.cfg.symbolDir {
		return nil, fmt.Errorf("got symbol dir %v; want %v", symDir, td.cfg.symbolDir)
	}
	needed, ok := td.dumps[path]
	if !ok {
		return nil, fmt.Errorf("unexpected dump %v", path)
	}

	used := make(breakpad.SymbolFileMap)
	missing = make(breakpad.SymbolFileMap)
	for p, id := range needed {
		if td.produced[p] == id {
			used[p] = id
		} else {
			missing[p] = id
		}
	}
	if _, err := io.WriteString(w, symbolFilesString(used)); err != nil {
		return nil, err
	}
	return missing, nil
}

// downloadSymbols implements Config.downloadSymbols.
// It checks whether the passed files are present in td.downloadable.
// Files that are present are added to td.produced; others are returned via missing.
func (td *testData) downloadSymbols(url, destDir string, files breakpad.SymbolFileMap) (missing breakpad.SymbolFileMap, err error) {
	td.downloadCalls++
	if destDir != td.cfg.symbolDir {
		return nil, fmt.Errorf("got dest dir %q; want %q", destDir, td.cfg.symbolDir)
	}
	return td.produce(files, td.downloadable), nil
}

// downloadSymbols implements Config.createSymbolFiles.
// It checks whether the passed files are present in td.creatable.
// Files that are present are added to td.produced; others are returned via missing.
func (td *testData) createSymbolFiles(cfg *Config, sf breakpad.SymbolFileMap) (missing breakpad.SymbolFileMap) {
	td.createCalls++
	return td.produce(sf, td.creatable)
}

// produce iterates through wanted. Any files that are present in available are added to td.produced;
// otherwise, they're reorted via missing. Shared implementation of downloadSymbols and createSymbolFiles.
func (td *testData) produce(wanted, available breakpad.SymbolFileMap) (missing breakpad.SymbolFileMap) {
	missing = make(breakpad.SymbolFileMap)
	for p, id := range wanted {
		if available[p] == id {
			td.produced[p] = id
		} else {
			missing[p] = id
		}
	}
	return missing
}

// symbolFilesString returns a sorted, space-separated list of "path|id" entries in files.
func symbolFilesString(files breakpad.SymbolFileMap) string {
	syms := make([]string, 0, len(files))
	for p, id := range files {
		syms = append(syms, fmt.Sprintf("%s|%s", p, id))
	}
	sort.Strings(syms)
	return strings.Join(syms, " ")
}

func TestSymbolizeCrash_DownloadOnly(t *testing.T) {
	td := newTestData(t)
	defer td.close()

	const (
		dump          = "1.dmp"
		download1Name = "downloaded1.sym"
		download1ID   = "1234"
		download2Name = "downloaded2.sym"
		download2ID   = "5678"
	)

	// Make the crash require two files, both of which can be downloaded.
	td.downloadable = breakpad.SymbolFileMap{
		download1Name: download1ID,
		download2Name: download2ID,
	}
	td.dumps = map[string]breakpad.SymbolFileMap{dump: td.downloadable}

	b := &bytes.Buffer{}
	if err := SymbolizeCrash(dump, b, td.cfg); err != nil {
		t.Error("SymbolizeCrash failed: ", err)
	}
	if exp := symbolFilesString(td.downloadable); b.String() != exp {
		t.Errorf("SymbolizeCrash used symbol files %q; want %q", b.String(), exp)
	}
	if td.downloadCalls != 1 {
		t.Errorf("SymbolizeCrash called downloadSymbols %d times; want 1", td.downloadCalls)
	}
	if td.createCalls != 0 { // not called since files were downloaded
		t.Errorf("SymbolizeCrash called createSymbolFiles %d time(s); want 0", td.createCalls)
	}
	if td.walkCalls != 2 { // once to find required symbols and once after downloading
		t.Errorf("SymbolizeCrash called walkMinidump %d time(s); want 2", td.walkCalls)
	}
}

func TestSymbolizeCrash_CreateAfterDownloadError(t *testing.T) {
	td := newTestData(t)
	defer td.close()

	const (
		dump       = "1.dmp"
		createName = "create.sym"
		createID   = "1234"
	)

	// If downloadSymbols reports an error, createSymbolFiles should still be called.
	td.downloadErr = errors.New("download failed")
	td.creatable = breakpad.SymbolFileMap{createName: createID}
	td.dumps = map[string]breakpad.SymbolFileMap{dump: td.creatable}

	b := &bytes.Buffer{}
	if err := SymbolizeCrash(dump, b, td.cfg); err != nil {
		t.Error("SymbolizeCrash failed: ", err)
	} else if exp := symbolFilesString(td.creatable); b.String() != exp {
		t.Errorf("SymbolizeCrash used symbol files %q; want %q", b.String(), exp)
	}
	if td.downloadCalls != 1 {
		t.Errorf("SymbolizeCrash called downloadSymbols %d times; want 1", td.downloadCalls)
	}
	if td.createCalls != 1 {
		t.Errorf("SymbolizeCrash called createSymbolFiles %d times; want 1", td.createCalls)
	}
	if td.walkCalls != 2 { // once to find required symbols and once after creating
		t.Errorf("SymbolizeCrash called walkMinidump %d time(s); want 2", td.walkCalls)
	}
}

func TestSymbolizeCrash_EmptyBuilderPath(t *testing.T) {
	td := newTestData(t)
	defer td.close()

	const (
		dump       = "1.dmp"
		createFile = "create.sym"
		createID   = "1234"
	)

	td.relInfo.builderPath = "" // simulate non-builder build
	td.creatable = breakpad.SymbolFileMap{createFile: createID}
	td.dumps = map[string]breakpad.SymbolFileMap{dump: td.creatable}

	b := &bytes.Buffer{}
	if err := SymbolizeCrash(dump, b, td.cfg); err != nil {
		t.Error("SymbolizeCrash failed: ", err)
	} else if exp := symbolFilesString(td.creatable); b.String() != exp {
		t.Errorf("SymbolizeCrash used symbol files %q; want %q", b.String(), exp)
	}
	if td.downloadCalls != 0 { // not called since builderPath is empty
		t.Errorf("SymbolizeCrash called downloadSymbols %d time(s); want 0", td.downloadCalls)
	}
	if td.createCalls != 1 {
		t.Errorf("SymbolizeCrash called createSymbolFiles %d times; want 1", td.createCalls)
	}
	if td.walkCalls != 2 { // once to find required symbols and once after creating
		t.Errorf("SymbolizeCrash called walkMinidump %d time(s); want 2", td.walkCalls)
	}
}

func TestSymbolizeCrash_ReuseSymbols(t *testing.T) {
	td := newTestData(t)
	defer td.close()

	const (
		dump1        = "1.dmp"
		dump2        = "2.dmp"
		downloadName = "downloaded.sym"
		downloadID   = "1234"
		createName   = "created.sym"
		createID     = "5678"
		missingName  = "missing.sym"
		missingID    = "9ABC"
	)

	td.downloadable = breakpad.SymbolFileMap{downloadName: downloadID}
	td.creatable = breakpad.SymbolFileMap{createName: createID}
	available := breakpad.SymbolFileMap{
		downloadName: downloadID,
		createName:   createID,
	}
	needed := breakpad.SymbolFileMap{
		downloadName: downloadID,
		createName:   createID,
		missingName:  missingID,
	}
	td.dumps = map[string]breakpad.SymbolFileMap{dump1: needed, dump2: needed}

	b := &bytes.Buffer{}
	if err := SymbolizeCrash(dump1, b, td.cfg); err != nil {
		t.Error("SymbolizeCrash failed: ", err)
	} else if exp := symbolFilesString(available); b.String() != exp {
		t.Errorf("SymbolizeCrash used symbol files %q; want %q", b.String(), exp)
	}
	if td.downloadCalls != 1 {
		t.Errorf("SymbolizeCrash called downloadSymbols %d times; want 1", td.downloadCalls)
	}
	if td.createCalls != 1 {
		t.Errorf("SymbolizeCrash called createSymbolFiles %d times; want 1", td.createCalls)
	}
	if td.walkCalls != 2 { // once to find required symbols and once after downloading/creating
		t.Errorf("SymbolizeCrash called walkMinidump %d times; want 1", td.walkCalls)
	}

	// Symbolize a second crash that needs the same symbol files and check that we don't
	// try to download or create symbols again, even though there's a missing symbol file
	// (since we already tried to get it previously).
	b.Reset()
	td.resetStats()
	if err := SymbolizeCrash(dump2, b, td.cfg); err != nil {
		t.Error("SymbolizeCrash failed: ", err)
	} else if exp := symbolFilesString(available); b.String() != exp {
		t.Errorf("SymbolizeCrash used symbol files %q; want %q", b.String(), exp)
	}
	if td.downloadCalls != 0 {
		t.Errorf("SymbolizeCrash called downloadSymbols %d time(s); want 0", td.downloadCalls)
	}
	if td.createCalls != 0 {
		t.Errorf("SymbolizeCrash called createSymbolFiles %d time(s); want 0", td.createCalls)
	}
	if td.walkCalls != 1 {
		t.Errorf("SymbolizeCrash called walkMinidump %d times; want 1", td.walkCalls)
	}
}
