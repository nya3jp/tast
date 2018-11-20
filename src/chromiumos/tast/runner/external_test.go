// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	gotesting "testing"

	"chromiumos/tast/devserver"
	"chromiumos/tast/testing"
	"chromiumos/tast/testutil"
)

func dummyLogFn(msg string) {}

// Simple scenario of one internal data file and two external data files.
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
				// extFile2 is not downloaded due to inconsistent link data.
			},
		},
	}
	if !reflect.DeepEqual(jobs, exp) {
		t.Errorf("prepareDownloads returned %v; want %v", jobs, exp)
	}
}

// Staleness is decided by file size and hash.
func TestPrepareDownloadsStale(t *gotesting.T) {
	const (
		pkg          = "cat"
		extFile1     = "ext_file1.txt"
		extFile2     = "ext_file2.txt"
		extData1     = "foo"
		extData2     = "bar"
		extLink1     = "ext_file1.txt.external"
		extLink2     = "ext_file2.txt.external"
		extLink1JSON = `{"url": "url1", "size": 9, "sha256sum": "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"}`
		extLink2JSON = `{"url": "url2", "size": 3, "sha256sum": "bbbb"}`
	)

	dataDir := testutil.TempDir(t)
	defer os.RemoveAll(dataDir)
	dataSubdir := filepath.Join(dataDir, pkg, "data")

	if err := testutil.WriteFiles(dataSubdir, map[string]string{
		extLink1: extLink1JSON,
		extLink2: extLink2JSON,
		extFile1: extData1,
		extFile2: extData2,
	}); err != nil {
		t.Fatal(err)
	}

	tests := []*testing.Test{
		{Pkg: pkg, Data: []string{extFile1, extFile2}},
	}
	jobs := prepareDownloads(dataDir, tests, dummyLogFn)

	exp := []*downloadJob{
		{
			link: externalLink{URL: "url1", Size: 9, SHA256Sum: "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"},
			dests: []string{
				filepath.Join(dataSubdir, extFile1),
			},
		},
		{
			link: externalLink{URL: "url2", Size: 3, SHA256Sum: "bbbb"},
			dests: []string{
				filepath.Join(dataSubdir, extFile2),
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
		extData1     = "foo"
		extData2     = "bar"
		extLink1     = "ext_file1.txt.external"
		extLink2     = "ext_file2.txt.external"
		extLink1JSON = `{"url": "url1", "size": 3, "sha256sum": "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"}`
		extLink2JSON = `{"url": "url2", "size": 3, "sha256sum": "fcde2b2edba56bf408601fb721fe9b5c338d10ee429ea04fae5511b68fbf8fb9"}`
	)

	dataDir := testutil.TempDir(t)
	defer os.RemoveAll(dataDir)
	dataSubdir := filepath.Join(dataDir, pkg, "data")

	if err := testutil.WriteFiles(dataSubdir, map[string]string{
		extLink1: extLink1JSON,
		extLink2: extLink2JSON,
		extFile1: extData1,
		extFile2: extData2,
	}); err != nil {
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

// All files are successfully downloaded.
func TestRunDownloadsSimple(t *gotesting.T) {
	const (
		file1      = "file1"
		file2      = "file2"
		url1       = "url1"
		url2       = "url2"
		data1      = "foo"
		data2      = "bar"
		sha256Sum1 = "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"
		sha256Sum2 = "fcde2b2edba56bf408601fb721fe9b5c338d10ee429ea04fae5511b68fbf8fb9"
	)
	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	jobs := []*downloadJob{
		{
			link: externalLink{
				URL:       url1,
				Size:      3,
				SHA256Sum: sha256Sum1,
			},
			dests: []string{filepath.Join(tmpDir, file1)},
		},
		{
			link: externalLink{
				URL:        url2,
				Size:       3,
				SHA256Sum:  sha256Sum2,
				Executable: true,
			},
			dests: []string{filepath.Join(tmpDir, file2)},
		},
	}
	cl := devserver.NewFakeClient(map[string][]byte{
		url1: []byte(data1),
		url2: []byte(data2),
	})

	runDownloads(context.Background(), tmpDir, jobs, cl, dummyLogFn)

	path1 := filepath.Join(tmpDir, file1)
	if out, err := ioutil.ReadFile(path1); err != nil {
		t.Error(err)
	} else if !bytes.Equal(out, []byte(data1)) {
		t.Errorf("Corrupted data for %s: got %q, want %q", file1, string(out), data1)
	}
	if fi, err := os.Stat(path1); err != nil {
		t.Error(err)
	} else if fi.Mode() != 0644 {
		t.Errorf("Unexpected mode for %s: got %o, want %o", file1, fi.Mode(), 0644)
	}

	path2 := filepath.Join(tmpDir, file2)
	if out, err := ioutil.ReadFile(path2); err != nil {
		t.Error(err)
	} else if !bytes.Equal(out, []byte(data2)) {
		t.Errorf("Corrupted data for %s: got %q, want %q", file2, string(out), data2)
	}
	if fi, err := os.Stat(path2); err != nil {
		t.Error(err)
	} else if fi.Mode() != 0755 {
		t.Errorf("Unexpected mode for %s: got %o, want %o", file2, fi.Mode(), 0755)
	}
}

// Corrupted downloads are not saved.
func TestRunDownloadsCorrupted(t *gotesting.T) {
	const (
		file1      = "file1"
		file2      = "file2"
		file3      = "file3"
		url1       = "url1"
		url2       = "url2"
		url3       = "url3"
		data1      = "foo"
		data2      = "bar"
		sha256Sum1 = "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"
		sha256Sum2 = "bbbb" // wrong SHA256
		sha256Sum3 = "xxxx"
	)
	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	jobs := []*downloadJob{
		{
			link: externalLink{
				URL:       url1,
				Size:      12345, // wrong size
				SHA256Sum: sha256Sum1,
			},
			dests: []string{filepath.Join(tmpDir, file1)},
		},
		{
			link: externalLink{
				URL:       url2,
				Size:      3,
				SHA256Sum: sha256Sum2,
			},
			dests: []string{filepath.Join(tmpDir, file2)},
		},
		{
			link: externalLink{
				URL:       url3,
				Size:      3,
				SHA256Sum: sha256Sum3,
			},
			dests: []string{filepath.Join(tmpDir, file3)},
		},
	}
	cl := devserver.NewFakeClient(map[string][]byte{
		url1: []byte(data1),
		url2: []byte(data2),
		// url3 returns an error.
	})

	runDownloads(context.Background(), tmpDir, jobs, cl, dummyLogFn)

	for _, name := range []string{file1, file2, file3} {
		if _, err := os.Stat(filepath.Join(tmpDir, name)); err == nil {
			t.Errorf("%s exists", name)
		}
	}
}
