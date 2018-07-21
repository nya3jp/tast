// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"chromiumos/tast/testutil"
)

func TestLoadExternalDataMap(t *testing.T) {
	const (
		conf      = "external_data.conf"
		path1     = "cat/data/file.txt"
		url1      = "gs://bogus-bucket/myfile.txt"
		path2     = "other/data/video.mp4"
		url2      = "gs://some-other-bucket/myvideo.mp4"
		bogusPath = "cat/data/bogus_file.txt"
	)

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	if err := testutil.WriteFiles(td, map[string]string{
		conf: fmt.Sprintf("# Here's a comment.\n\n%s %s\n %s %s \n", path1, url1, path2, url2),
	}); err != nil {
		t.Fatal(err)
	}

	cp := filepath.Join(td, conf)
	m, err := newExternalDataMap(cp)
	if err != nil {
		t.Fatalf("newExternalDataMap(%q) failed: %v", cp, err)
	}

	if fn := m.localFile(path1); fn == "" {
		t.Errorf("localFile(%q) returned empty filename", path1)
	}
	if fn := m.localFile(path2); fn == "" {
		t.Errorf("localFile(%q) returned empty filename", path2)
	}
	if fn := m.localFile(bogusPath); fn != "" {
		t.Errorf("localFile(%q) = %q; want empty", bogusPath, fn)
	}
}

func TestFetchExternalData(t *testing.T) {
	const (
		conf = "external_data.conf"

		file1 = "cat/data/file.txt"
		url1  = "my/data/foo.txt"
		data1 = "Here's some text."

		file2 = "cat2/data/file.txt"
		url2  = "more/data/foo.txt" // different file, but same basename as url1
		data2 = "Here's more text."

		file3 = "cat/data/other_file.txt" // in config file, but not requested later
		url3  = "my/data/foo2.txt"
		data3 = "Here's still more text."
	)

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	if err := testutil.WriteFiles(td, map[string]string{
		conf: fmt.Sprintf("%s %s\n%s %s\n%s %s\n",
			file1, filepath.Join(td, url1), file2, filepath.Join(td, url2), file3, filepath.Join(td, url3)),
		url1: data1,
		url2: data2,
	}); err != nil {
		t.Fatal(err)
	}

	cp := filepath.Join(td, conf)
	m, err := newExternalDataMap(cp)
	if err != nil {
		t.Fatalf("newExternalDataMap(%q) failed: %v", cp, err)
	}

	dd := filepath.Join(td, "data")
	files := []string{file1, file2, "/cat/data/some_local_file.txt"}
	if err = m.fetchFiles(files, dd, nil); err != nil {
		t.Fatalf("fetchFiles(%v, %q, nil) failed: %v", files, dd, err)
	}

	lf1 := m.localFile(file1)
	lf2 := m.localFile(file2)
	if act, err := testutil.ReadFiles(dd); err != nil {
		t.Error(err)
	} else if exp := map[string]string{lf1: data1, lf2: data2}; !reflect.DeepEqual(act, exp) {
		t.Errorf("fetchFiles(%v, %q) wrote %v; want %v", files, dd, act, exp)
	}
}

func TestLoadBadExternalDataMap(t *testing.T) {
	const conf = "external_data.conf"

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)
	for _, data := range []string{
		"blah",  // single value
		"a b c", // three values
	} {
		if err := testutil.WriteFiles(td, map[string]string{conf: data}); err != nil {
			t.Fatal(err)
		}
		_, err := newExternalDataMap(filepath.Join(td, conf))
		if err == nil {
			t.Fatalf("newExternalDataMap() didn't fail for %q", data)
		}
	}

}
