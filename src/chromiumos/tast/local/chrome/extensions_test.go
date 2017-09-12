// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package chrome

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"chromiumos/tast/common/testutil"
)

func TestGetExtensionDirs(t *testing.T) {
	td, err := ioutil.TempDir("", "extensions_test.")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(td)

	if err := os.Mkdir(filepath.Join(td, "empty"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteFiles(td, map[string]string{
		"invalid/some_other_file.json": "{}",
		"valid/manifest.json":          "{}",
	}); err != nil {
		t.Fatal(err)
	}

	act, err := getExtensionDirs(td)
	if err != nil {
		t.Fatalf("getExtensionDirs(%q) failed with %v", td, err)
	}
	exp := []string{filepath.Join(td, "valid")}
	if !reflect.DeepEqual(act, exp) {
		t.Fatalf("getExtensionDirs(%q) = %v; want %v", td, act, exp)
	}
}

func TestComputeExtensionIdFromPublicKey(t *testing.T) {
	dir, err := ioutil.TempDir("", "extensions_test.")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Taken from Chrome's components/crx_file/id_util_unittest.cc.
	manifest := `{ "key": "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQC4fysg3HybDNxRYZkNNg/UZogIVYTVOr8rpGSFewwEEz+N9Lw4DUn+a8RasEBTOtdmCQ+eNnQw2ooxTx8UUNfHIJQX3k65V15+CuWyZXqJTrZH/xy9tzgTr0eFhDIz8xdJv+mW0NYUbxONxfwscrqs6n4YU1amg6LOk5PnHw/mDwIDAQAB" }`
	if err := ioutil.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	id, err := computeExtensionId(dir)
	if err != nil {
		t.Fatalf("computeExtensionId(%q) failed with %v", dir, err)
	}
	exp := "melddjfinppjdikinhbgehiennejpfhp"
	if id != exp {
		t.Errorf("computeExtensionId(%q) = %q; want %q", dir, id, exp)
	}
}

func TestComputeExtensionIdFromDirName(t *testing.T) {
	// Taken from Chrome's components/crx_file/id_util_unittest.cc.
	for _, tc := range []struct{ dir, exp string }{
		{"test", "jpignaibiiemhngfjkcpokkamffknabf"},
		{"_", "ncocknphbhhlhkikpnnlmbcnbgdempcd"},
	} {
		if id, err := computeExtensionId(tc.dir); err != nil {
			t.Errorf("computeExtensionId(%q) failed with %v", tc.dir, err)
		} else if id != tc.exp {
			t.Errorf("computeExtensionId(%q) = %q; want %q", tc.dir, id, tc.exp)
		}
	}
}
