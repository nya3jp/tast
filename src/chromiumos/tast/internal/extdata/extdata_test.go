// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package extdata

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/devserver"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/testutil"
)

const fakeArtifactURL = "gs://somebucket/path/to/artifacts/"

// Simple scenario of one internal data file and two static external data files.
func TestPrepareDownloadsStatic(t *gotesting.T) {
	const (
		pkg          = "cat"
		intFile      = "int_file.txt"
		extFile1     = "ext_file1.txt"
		extFile2     = "ext_file2.txt"
		extLink1     = extFile1 + testing.ExternalLinkSuffix
		extLink2     = extFile2 + testing.ExternalLinkSuffix
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

	tests := []*protocol.Entity{
		{Package: pkg, Dependencies: &protocol.EntityDependencies{DataFiles: []string{extFile1}}},
		{Package: pkg, Dependencies: &protocol.EntityDependencies{DataFiles: []string{intFile, extFile2}}},
	}
	ctx := context.Background()
	m, err := NewManager(ctx, dataDir, fakeArtifactURL)
	if err != nil {
		t.Fatal(err)
	}
	jobs, _ := m.PrepareDownloads(ctx, tests)

	exp := []*DownloadJob{
		{
			link:  &link{Data: LinkData{StaticURL: "url1", Size: 111, SHA256Sum: "aaaa", Executable: false}, ComputedURL: "url1"},
			dests: []string{filepath.Join(dataSubdir, extFile1)},
		},
		{
			link:  &link{Data: LinkData{StaticURL: "url2", Size: 222, SHA256Sum: "bbbb", Executable: true}, ComputedURL: "url2"},
			dests: []string{filepath.Join(dataSubdir, extFile2)},
		},
	}
	if !reflect.DeepEqual(jobs, exp) {
		t.Errorf("PrepareDownloads returned %v; want %v", jobs, exp)
	}
}

// Simple scenario of one internal data file and two artifact external data files.
func TestPrepareDownloadsArtifact(t *gotesting.T) {
	const (
		pkg          = "cat"
		intFile      = "int_file.txt"
		extFile1     = "ext_file1.txt"
		extFile2     = "ext_file2.txt"
		extLink1     = extFile1 + testing.ExternalLinkSuffix
		extLink2     = extFile2 + testing.ExternalLinkSuffix
		extLink1JSON = `{"type": "artifact", "name": "some_artifact1"}`
		extLink2JSON = `{"type": "artifact", "name": "some_artifact2", "executable": true}`
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

	tests := []*protocol.Entity{
		{Package: pkg, Dependencies: &protocol.EntityDependencies{DataFiles: []string{extFile1}}},
		{Package: pkg, Dependencies: &protocol.EntityDependencies{DataFiles: []string{intFile, extFile2}}},
	}
	ctx := context.Background()
	m, err := NewManager(ctx, dataDir, fakeArtifactURL)
	if err != nil {
		t.Fatal(err)
	}
	jobs, _ := m.PrepareDownloads(ctx, tests)

	exp := []*DownloadJob{
		{
			link:  &link{Data: LinkData{Type: TypeArtifact, Name: "some_artifact1"}, ComputedURL: fakeArtifactURL + "some_artifact1"},
			dests: []string{filepath.Join(dataSubdir, extFile1)},
		},
		{
			link:  &link{Data: LinkData{Type: TypeArtifact, Name: "some_artifact2", Executable: true}, ComputedURL: fakeArtifactURL + "some_artifact2"},
			dests: []string{filepath.Join(dataSubdir, extFile2)},
		},
	}
	if !reflect.DeepEqual(jobs, exp) {
		t.Errorf("PrepareDownloads returned %v; want %v", jobs, exp)
	}
}

// Duplicated links should be consolidated into one download.
func TestPrepareDownloadsDupLinks(t *gotesting.T) {
	const (
		pkg         = "cat"
		extFile1    = "ext_file1.txt"
		extFile2    = "ext_file2.txt"
		extFile3    = "ext_file3.txt"
		extLink1    = extFile1 + testing.ExternalLinkSuffix
		extLink2    = extFile2 + testing.ExternalLinkSuffix
		extLink3    = extFile3 + testing.ExternalLinkSuffix
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

	tests := []*protocol.Entity{
		{Package: pkg, Dependencies: &protocol.EntityDependencies{DataFiles: []string{extFile1, extFile2}}},
		{Package: pkg, Dependencies: &protocol.EntityDependencies{DataFiles: []string{extFile2, extFile3}}},
	}

	ctx := context.Background()
	m, err := NewManager(ctx, dataDir, fakeArtifactURL)
	if err != nil {
		t.Fatal(err)
	}
	jobs, _ := m.PrepareDownloads(ctx, tests)

	exp := []*DownloadJob{
		{
			link: &link{Data: LinkData{StaticURL: "url1", Size: 111, SHA256Sum: "aaaa", Executable: false}, ComputedURL: "url1"},
			dests: []string{
				filepath.Join(dataSubdir, extFile1),
				filepath.Join(dataSubdir, extFile2),
				filepath.Join(dataSubdir, extFile3),
			},
		},
	}
	if !reflect.DeepEqual(jobs, exp) {
		t.Errorf("PrepareDownloads returned %v; want %v", jobs, exp)
	}
}

// Duplicated links should have consistent link data.
func TestPrepareDownloadsInconsistentDupLinks(t *gotesting.T) {
	const (
		pkg          = "cat"
		extFile1     = "ext_file1.txt"
		extFile2     = "ext_file2.txt"
		extLink1     = extFile1 + testing.ExternalLinkSuffix
		extLink2     = extFile2 + testing.ExternalLinkSuffix
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

	tests := []*protocol.Entity{
		{Package: pkg, Dependencies: &protocol.EntityDependencies{DataFiles: []string{extFile1, extFile2}}},
	}
	ctx := context.Background()
	m, err := NewManager(ctx, dataDir, fakeArtifactURL)
	if err != nil {
		t.Fatal(err)
	}
	jobs, _ := m.PrepareDownloads(ctx, tests)

	exp := []*DownloadJob{
		{
			link: &link{Data: LinkData{StaticURL: "same_url", Size: 111, SHA256Sum: "aaaa", Executable: false}, ComputedURL: "same_url"},
			dests: []string{
				filepath.Join(dataSubdir, extFile1),
				// extFile2 is not downloaded due to inconsistent link data.
			},
		},
	}
	if !reflect.DeepEqual(jobs, exp) {
		t.Errorf("PrepareDownloads returned %v; want %v", jobs, exp)
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
		extLink1     = extFile1 + testing.ExternalLinkSuffix
		extLink2     = extFile2 + testing.ExternalLinkSuffix
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

	tests := []*protocol.Entity{
		{Package: pkg, Dependencies: &protocol.EntityDependencies{DataFiles: []string{extFile1, extFile2}}},
	}
	ctx := context.Background()
	m, err := NewManager(ctx, dataDir, fakeArtifactURL)
	if err != nil {
		t.Fatal(err)
	}
	jobs, _ := m.PrepareDownloads(ctx, tests)

	exp := []*DownloadJob{
		{
			link: &link{Data: LinkData{StaticURL: "url1", Size: 9, SHA256Sum: "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"}, ComputedURL: "url1"},
			dests: []string{
				filepath.Join(dataSubdir, extFile1),
			},
		},
		{
			link: &link{Data: LinkData{StaticURL: "url2", Size: 3, SHA256Sum: "bbbb"}, ComputedURL: "url2"},
			dests: []string{
				filepath.Join(dataSubdir, extFile2),
			},
		},
	}
	if !reflect.DeepEqual(jobs, exp) {
		t.Errorf("PrepareDownloads returned %v; want %v", jobs, exp)
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
		extLink1     = extFile1 + testing.ExternalLinkSuffix
		extLink2     = extFile2 + testing.ExternalLinkSuffix
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

	tests := []*protocol.Entity{
		{Package: pkg, Dependencies: &protocol.EntityDependencies{DataFiles: []string{extFile1, extFile2}}},
	}
	ctx := context.Background()
	m, err := NewManager(ctx, dataDir, fakeArtifactURL)
	if err != nil {
		t.Fatal(err)
	}
	jobs, _ := m.PrepareDownloads(ctx, tests)

	if len(jobs) > 0 {
		t.Errorf("PrepareDownloads returned %v; want []", jobs)
	}
}

// Broken links are ignored.
func TestPrepareDownloadsBrokenLink(t *gotesting.T) {
	const (
		pkg          = "cat"
		extFile1     = "ext_file1.txt"
		extFile2     = "ext_file2.txt"
		extFile3     = "ext_file3.txt"
		extLink1     = extFile1 + testing.ExternalLinkSuffix
		extLink2     = extFile2 + testing.ExternalLinkSuffix
		extLink3     = extFile3 + testing.ExternalLinkSuffix
		extLink1JSON = "Hello, world!"                                     // not JSON
		extLink2JSON = `{"url": "url2", "size": 222, "sha256sum": "bbbb"}` // OK
		extLink3JSON = `{"type": "artifact", "name": "foo", "size": 123}`  // size must be omitted
	)

	dataDir := testutil.TempDir(t)
	defer os.RemoveAll(dataDir)
	dataSubdir := filepath.Join(dataDir, pkg, "data")

	if err := testutil.WriteFiles(dataSubdir, map[string]string{
		extLink1: extLink1JSON,
		extLink2: extLink2JSON,
		extLink3: extLink3JSON,
	}); err != nil {
		t.Fatal(err)
	}

	tests := []*protocol.Entity{
		{Package: pkg, Dependencies: &protocol.EntityDependencies{DataFiles: []string{extFile1, extFile2}}},
	}
	ctx := context.Background()
	m, err := NewManager(ctx, dataDir, fakeArtifactURL)
	if err != nil {
		t.Fatal(err)
	}
	jobs, _ := m.PrepareDownloads(ctx, tests)

	exp := []*DownloadJob{
		{
			link: &link{Data: LinkData{StaticURL: "url2", Size: 222, SHA256Sum: "bbbb", Executable: false}, ComputedURL: "url2"},
			dests: []string{
				filepath.Join(dataSubdir, extFile2),
			},
		},
	}
	if !reflect.DeepEqual(jobs, exp) {
		t.Errorf("PrepareDownloads returned %v; want %v", jobs, exp)
	}
}

// Artifact links can not be resolved if artifactURL is unavailable.
func TestPrepareDownloadsArtifactUnavailable(t *gotesting.T) {
	const (
		pkg          = "cat"
		extFile1     = "ext_file1.txt"
		extLink1     = extFile1 + testing.ExternalLinkSuffix
		extLink1JSON = `{"type": "artifact", "name": "foo"}`
	)

	dataDir := testutil.TempDir(t)
	defer os.RemoveAll(dataDir)
	dataSubdir := filepath.Join(dataDir, pkg, "data")

	if err := testutil.WriteFiles(dataSubdir, map[string]string{
		extLink1: extLink1JSON,
	}); err != nil {
		t.Fatal(err)
	}

	tests := []*protocol.Entity{
		{Package: pkg, Dependencies: &protocol.EntityDependencies{DataFiles: []string{extFile1}}},
	}
	ctx := context.Background()
	m, err := NewManager(ctx, dataDir, "")
	if err != nil {
		t.Fatal(err)
	}
	jobs, _ := m.PrepareDownloads(ctx, tests)

	if len(jobs) > 0 {
		t.Errorf("PrepareDownloads returned %v; want []", jobs)
	}
}

// Errors are written to files.
func TestPrepareDownloadsError(t *gotesting.T) {
	const (
		pkg          = "cat"
		extFile1     = "ext_file1.txt"
		extFile2     = "ext_file2.txt"
		extLink1     = extFile1 + testing.ExternalLinkSuffix
		extLink2     = extFile2 + testing.ExternalLinkSuffix
		extError1    = extFile1 + testing.ExternalErrorSuffix
		extError2    = extFile2 + testing.ExternalErrorSuffix
		extLink1JSON = "Hello, world!"
		extLink2JSON = `{"url": "url2", "size": 222, "sha256sum": "bbbb"}`
	)

	dataDir := testutil.TempDir(t)
	defer os.RemoveAll(dataDir)
	dataSubdir := filepath.Join(dataDir, pkg, "data")

	if err := testutil.WriteFiles(dataSubdir, map[string]string{
		extLink1:  extLink1JSON,
		extLink2:  extLink2JSON,
		extError2: "previous error",
	}); err != nil {
		t.Fatal(err)
	}

	tests := []*protocol.Entity{
		{Package: pkg, Dependencies: &protocol.EntityDependencies{DataFiles: []string{extFile1, extFile2}}},
	}
	ctx := context.Background()
	m, err := NewManager(ctx, dataDir, fakeArtifactURL)
	if err != nil {
		t.Fatal(err)
	}
	m.PrepareDownloads(ctx, tests)

	// extError1 should exist due to JSON parse error.
	if _, err := os.Stat(filepath.Join(dataSubdir, extError1)); err != nil {
		t.Errorf("%s not found; expected to exist", extError1)
	}

	// extError2 should not exist.
	if _, err := os.Stat(filepath.Join(dataSubdir, extError2)); err == nil {
		t.Errorf("%s exists; expected to be deleted", extError2)
	}
}

// Tests purgeable computation.
func TestPrepareDownloadsPurgeable(t *gotesting.T) {
	const (
		pkg     = "cat"
		extData = "dataA"
		extLink = `{"url": "urlA", "size": 5, "sha256sum": "7e45b6647a5f7c34a572ca8a585c5c0703ab40152fbf9158e30bff7f339d64ac"}`
	)

	dataDir := testutil.TempDir(t)
	defer os.RemoveAll(dataDir)
	dataSubdir := filepath.Join(dataDir, pkg, "data")
	if err := testutil.WriteFiles(dataSubdir, map[string]string{
		"int_file1.txt": "data1", // internal, used
		"int_file2.txt": "data2", // internal, unused

		"ext_file1.txt":          extData, // correct, used
		"ext_file1.txt.external": extLink,

		"ext_file2.txt":          extData, // correct, unused
		"ext_file2.txt.external": extLink,

		"ext_file3.txt.external": extLink, // to be downloaded

		"ext_file4.txt":          "broken", // broken, used
		"ext_file4.txt.external": extLink,

		"ext_file5.txt":          "broken", // broken, unused
		"ext_file5.txt.external": extLink,
	}); err != nil {
		t.Fatal(err)
	}

	tests := []*protocol.Entity{{
		Package: pkg,
		Dependencies: &protocol.EntityDependencies{DataFiles: []string{
			"int_file1.txt",
			// int_file2.txt is not used.
			"ext_file1.txt",
			// ext_file2.txt is not used.
			"ext_file3.txt",
			"ext_file4.txt",
			// ext_file5.txt is not used.
			"ext_file6.txt",
		}},
	}}

	ctx := context.Background()
	m, err := NewManager(ctx, dataDir, fakeArtifactURL)
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(m.Purgeable(), []string{
		filepath.Join(dataSubdir, "ext_file1.txt"),
		filepath.Join(dataSubdir, "ext_file2.txt"),
		filepath.Join(dataSubdir, "ext_file4.txt"),
		filepath.Join(dataSubdir, "ext_file5.txt"),
	}); diff != "" {
		t.Error("Purgeable mismatch (-got +want):\n", diff)
	}

	_, release := m.PrepareDownloads(ctx, tests)

	if diff := cmp.Diff(m.Purgeable(), []string{
		filepath.Join(dataSubdir, "ext_file2.txt"),
		filepath.Join(dataSubdir, "ext_file5.txt"),
	}); diff != "" {
		t.Error("Purgeable mismatch after PrepareDownloads (-got +want):\n", diff)
	}

	release()

	if diff := cmp.Diff(m.Purgeable(), []string{
		filepath.Join(dataSubdir, "ext_file1.txt"),
		filepath.Join(dataSubdir, "ext_file2.txt"),
		// ext_file4.txt was found to be broken, and removed.
		filepath.Join(dataSubdir, "ext_file5.txt"),
	}); diff != "" {
		t.Error("Purgeable mismatch after release (-got +want):\n", diff)
	}
}

// Static external data files are successfully downloaded.
func TestRunDownloadsStatic(t *gotesting.T) {
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

	jobs := []*DownloadJob{
		{
			link: &link{
				Data: LinkData{
					StaticURL: url1,
					Size:      3,
					SHA256Sum: sha256Sum1,
				},
				ComputedURL: url1,
			},
			dests: []string{filepath.Join(tmpDir, file1)},
		},
		{
			link: &link{
				Data: LinkData{
					StaticURL:  url2,
					Size:       3,
					SHA256Sum:  sha256Sum2,
					Executable: true,
				},
				ComputedURL: url2,
			},
			dests: []string{filepath.Join(tmpDir, file2)},
		},
	}
	cl := devserver.NewFakeClient(map[string][]byte{
		url1: []byte(data1),
		url2: []byte(data2),
	})

	RunDownloads(context.Background(), tmpDir, jobs, cl)

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

// Artifact external data files are successfully downloaded.
func TestRunDownloadsArtifact(t *gotesting.T) {
	const (
		name1 = "name1"
		name2 = "name2"
		file1 = "file1"
		file2 = "file2"
		url1  = fakeArtifactURL + name1
		url2  = fakeArtifactURL + name2
		data1 = "foo"
		data2 = "bar"
	)
	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	jobs := []*DownloadJob{
		{
			link: &link{
				Data: LinkData{
					Type: TypeArtifact,
					Name: name1,
				},
				ComputedURL: url1,
			},
			dests: []string{filepath.Join(tmpDir, file1)},
		},
		{
			link: &link{
				Data: LinkData{
					Type:       TypeArtifact,
					Name:       name2,
					Executable: true,
				},
				ComputedURL: url2,
			},
			dests: []string{filepath.Join(tmpDir, file2)},
		},
	}
	cl := devserver.NewFakeClient(map[string][]byte{
		url1: []byte(data1),
		url2: []byte(data2),
	})

	RunDownloads(context.Background(), tmpDir, jobs, cl)

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

	jobs := []*DownloadJob{
		{
			link: &link{
				Data: LinkData{
					StaticURL: url1,
					Size:      12345, // wrong size
					SHA256Sum: sha256Sum1,
				},
				ComputedURL: url1,
			},
			dests: []string{filepath.Join(tmpDir, file1)},
		},
		{
			link: &link{
				Data: LinkData{
					StaticURL: url2,
					Size:      3,
					SHA256Sum: sha256Sum2,
				},
				ComputedURL: url2,
			},
			dests: []string{filepath.Join(tmpDir, file2)},
		},
		{
			link: &link{
				Data: LinkData{
					StaticURL: url3,
					Size:      3,
					SHA256Sum: sha256Sum3,
				},
				ComputedURL: url3,
			},
			dests: []string{filepath.Join(tmpDir, file3)},
		},
	}
	cl := devserver.NewFakeClient(map[string][]byte{
		url1: []byte(data1),
		url2: []byte(data2),
		// url3 returns an error.
	})

	RunDownloads(context.Background(), tmpDir, jobs, cl)

	for _, name := range []string{file1, file2, file3} {
		if _, err := os.Stat(filepath.Join(tmpDir, name)); err == nil {
			t.Errorf("%s exists", name)
		}
	}
}

// Errors are written to files.
func TestRunDownloadsError(t *gotesting.T) {
	const (
		file1     = "file1"
		file2     = "file2"
		url       = "url"
		data      = "foo"
		sha256Sum = "xxxx" // wrong sha256
	)
	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	jobs := []*DownloadJob{
		{
			link: &link{
				Data: LinkData{
					StaticURL: url,
					Size:      3,
					SHA256Sum: sha256Sum,
				},
				ComputedURL: url,
			},
			dests: []string{filepath.Join(tmpDir, file1)},
		},
		{
			link: &link{
				Data: LinkData{
					StaticURL: url,
					Size:      3,
					SHA256Sum: sha256Sum,
				},
				ComputedURL: url,
			},
			dests: []string{filepath.Join(tmpDir, file2)},
		},
	}
	cl := devserver.NewFakeClient(map[string][]byte{url: []byte(data)})

	RunDownloads(context.Background(), tmpDir, jobs, cl)

	for _, f := range []string{file1, file2} {
		path := filepath.Join(tmpDir, f+testing.ExternalErrorSuffix)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s does not exist", path)
		}
	}
}
