// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"os"
	"path/filepath"
	"reflect"
	gotesting "testing"
	"time"

	"chromiumos/tast/testing"
	"chromiumos/tast/testutil"
)

func dummyLogFn(msg string) {}

// Simple scenario of one internal data and two external data.
func TestPrepareDownloadsSimple(t *gotesting.T) {
	const (
		pkg          = "cat"
		intFile      = "int_file.txt"
		extFile1     = "ext_file1.txt"
		extFile2     = "ext_file2.txt"
		extLink1     = "ext_file1.txt.external"
		extLink2     = "ext_file2.txt.external"
		extLink1JSON = `{"url": "url1", "size": 111, "sha256sum": "aaaa"}`
		extLink2JSON = `{"url": "url2", "size": 222, "sha256sum": "bbbb", "executable": true}`
	)

	dataDir := testutil.TempDir(t)
	defer os.RemoveAll(dataDir)
	dataSubdir := filepath.Join(dataDir, pkg, "data")

	if err := testutil.WriteFiles(dataSubdir, map[string]string{
		intFile:  intFile,
		extLink1: extLink1JSON,
		extLink2: extLink2JSON,
	}); err != nil {
		t.Fatal(err)
	}

	tests := []*testing.Test{
		{Pkg: pkg, Data: []string{extFile1}},
		{Pkg: pkg, Data: []string{intFile, extFile2}},
	}
	jobs := prepareDownloads(dataDir, tests, dummyLogFn)

	exp := []*downloadJob{
		{
			link:  externalLink{URL: "url1", Size: 111, SHA256Sum: "aaaa", Executable: false},
			dests: []string{filepath.Join(dataSubdir, extFile1)},
		},
		{
			link:  externalLink{URL: "url2", Size: 222, SHA256Sum: "bbbb", Executable: true},
			dests: []string{filepath.Join(dataSubdir, extFile2)},
		},
	}
	if !reflect.DeepEqual(jobs, exp) {
		t.Errorf("prepareDownloads returned %v; want %v", jobs, exp)
	}
}

// Duplicated links should be consolidated into one download.
func TestPrepareDownloadsDupLinks(t *gotesting.T) {
	const (
		pkg         = "cat"
		extFile1    = "ext_file1.txt"
		extFile2    = "ext_file2.txt"
		extFile3    = "ext_file3.txt"
		extLink1    = "ext_file1.txt.external"
		extLink2    = "ext_file2.txt.external"
		extLink3    = "ext_file3.txt.external"
		extLinkJSON = `{"url": "url1", "size": 111, "sha256sum": "aaaa"}`
	)

	dataDir := testutil.TempDir(t)
	defer os.RemoveAll(dataDir)
	dataSubdir := filepath.Join(dataDir, pkg, "data")

	if err := testutil.WriteFiles(dataSubdir, map[string]string{
		extLink1: extLinkJSON,
		extLink2: extLinkJSON,
		extLink3: extLinkJSON,
	}); err != nil {
		t.Fatal(err)
	}

	tests := []*testing.Test{
		{Pkg: pkg, Data: []string{extFile1, extFile2}},
		{Pkg: pkg, Data: []string{extFile2, extFile3}},
	}
	jobs := prepareDownloads(dataDir, tests, dummyLogFn)

	exp := []*downloadJob{
		{
			link: externalLink{URL: "url1", Size: 111, SHA256Sum: "aaaa", Executable: false},
			dests: []string{
				filepath.Join(dataSubdir, extFile1),
				filepath.Join(dataSubdir, extFile2),
				filepath.Join(dataSubdir, extFile3),
			},
		},
	}
	if !reflect.DeepEqual(jobs, exp) {
		t.Errorf("prepareDownloads returned %v; want %v", jobs, exp)
	}
}

// Duplicated links should have consistent link data.
func TestPrepareDownloadsInconsistentDupLinks(t *gotesting.T) {
	const (
		pkg          = "cat"
		extFile1     = "ext_file1.txt"
		extFile2     = "ext_file2.txt"
		extLink1     = "ext_file1.txt.external"
		extLink2     = "ext_file2.txt.external"
		extLink1JSON = `{"url": "same_url", "size": 111, "sha256sum": "aaaa"}`
		extLink2JSON = `{"url": "same_url", "size": 222, "sha256sum": "aaaa"}`
	)

	dataDir := testutil.TempDir(t)
	defer os.RemoveAll(dataDir)
	dataSubdir := filepath.Join(dataDir, pkg, "data")

	if err := testutil.WriteFiles(dataSubdir, map[string]string{
		extLink1: extLink1JSON,
		extLink2: extLink2JSON,
	}); err != nil {
		t.Fatal(err)
	}

	tests := []*testing.Test{
		{Pkg: pkg, Data: []string{extFile1, extFile2}},
	}
	jobs := prepareDownloads(dataDir, tests, dummyLogFn)

	exp := []*downloadJob{
		{
			link: externalLink{URL: "same_url", Size: 111, SHA256Sum: "aaaa", Executable: false},
			dests: []string{
				filepath.Join(dataSubdir, extFile1),
				// extFile2 is not downloaded due to inconsitent link data.
			},
		},
	}
	if !reflect.DeepEqual(jobs, exp) {
		t.Errorf("prepareDownloads returned %v; want %v", jobs, exp)
	}
}

// Staleness is decided by timestamps.
func TestPrepareDownloadsStale(t *gotesting.T) {
	const (
		pkg         = "cat"
		extFile1    = "ext_file1.txt"
		extFile2    = "ext_file2.txt"
		extLink1    = "ext_file1.txt.external"
		extLink2    = "ext_file2.txt.external"
		extLinkJSON = `{"url": "url1", "size": 111, "sha256sum": "aaaa"}`
	)

	dataDir := testutil.TempDir(t)
	defer os.RemoveAll(dataDir)
	dataSubdir := filepath.Join(dataDir, pkg, "data")

	if err := testutil.WriteFiles(dataSubdir, map[string]string{
		extLink1: extLinkJSON,
		extLink2: extLinkJSON,
		extFile1: extFile1,
		extFile2: extFile2,
	}); err != nil {
		t.Fatal(err)
	}

	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dataSubdir, extFile1), past, past); err != nil {
		t.Fatal(err)
	}

	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(dataSubdir, extFile2), future, future); err != nil {
		t.Fatal(err)
	}

	tests := []*testing.Test{
		{Pkg: pkg, Data: []string{extFile1, extFile2}},
	}
	jobs := prepareDownloads(dataDir, tests, dummyLogFn)

	exp := []*downloadJob{
		{
			link: externalLink{URL: "url1", Size: 111, SHA256Sum: "aaaa", Executable: false},
			dests: []string{
				// Only extFile1 is downloaded due to old timestamp.
				filepath.Join(dataSubdir, extFile1),
			},
		},
	}
	if !reflect.DeepEqual(jobs, exp) {
		t.Errorf("prepareDownloads returned %v; want %v", jobs, exp)
	}
}

// All files are up-to-date.
func TestPrepareDownloadsUpToDate(t *gotesting.T) {
	const (
		pkg          = "cat"
		extFile1     = "ext_file1.txt"
		extFile2     = "ext_file2.txt"
		extLink1     = "ext_file1.txt.external"
		extLink2     = "ext_file2.txt.external"
		extLink1JSON = `{"url": "url1", "size": 111, "sha256sum": "aaaa"}`
		extLink2JSON = `{"url": "url2", "size": 222, "sha256sum": "bbbb"}`
	)

	dataDir := testutil.TempDir(t)
	defer os.RemoveAll(dataDir)
	dataSubdir := filepath.Join(dataDir, pkg, "data")

	if err := testutil.WriteFiles(dataSubdir, map[string]string{
		extLink1: extLink1JSON,
		extLink2: extLink2JSON,
		extFile1: extFile1,
		extFile2: extFile2,
	}); err != nil {
		t.Fatal(err)
	}

	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(dataSubdir, extFile1), future, future); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(dataSubdir, extFile2), future, future); err != nil {
		t.Fatal(err)
	}

	tests := []*testing.Test{
		{Pkg: pkg, Data: []string{extFile1, extFile2}},
	}
	jobs := prepareDownloads(dataDir, tests, dummyLogFn)

	if len(jobs) > 0 {
		t.Errorf("prepareDownloads returned %v; want []", jobs)
	}
}

// Broken links are ignored.
func TestPrepareDownloadsBrokenLink(t *gotesting.T) {
	const (
		pkg          = "cat"
		extFile1     = "ext_file1.txt"
		extFile2     = "ext_file2.txt"
		extLink1     = "ext_file1.txt.external"
		extLink2     = "ext_file2.txt.external"
		extLink1JSON = "Hello, world!"
		extLink2JSON = `{"url": "url2", "size": 222, "sha256sum": "bbbb"}`
	)

	dataDir := testutil.TempDir(t)
	defer os.RemoveAll(dataDir)
	dataSubdir := filepath.Join(dataDir, pkg, "data")

	if err := testutil.WriteFiles(dataSubdir, map[string]string{
		extLink1: extLink1JSON,
		extLink2: extLink2JSON,
	}); err != nil {
		t.Fatal(err)
	}

	tests := []*testing.Test{
		{Pkg: pkg, Data: []string{extFile1, extFile2}},
	}
	jobs := prepareDownloads(dataDir, tests, dummyLogFn)

	exp := []*downloadJob{
		{
			link: externalLink{URL: "url2", Size: 222, SHA256Sum: "bbbb", Executable: false},
			dests: []string{
				filepath.Join(dataSubdir, extFile2),
			},
		},
	}
	if !reflect.DeepEqual(jobs, exp) {
		t.Errorf("prepareDownloads returned %v; want %v", jobs, exp)
	}
}
