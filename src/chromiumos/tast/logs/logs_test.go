// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logs

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"chromiumos/tast/testutil"
)

// getUpdates passes sizes to CopyLogFileUpdates to get file updates within dir
// and then returns the copied data as a map from relative filename to content,
// along with a set containing all copied relative paths (including empty files and broken symlinks).
func getUpdates(dir string, sizes InodeSizes) (updates map[string]string, paths map[string]struct{}, err error) {
	var dest string
	if dest, err = ioutil.TempDir("", "tast_logs."); err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(dest)

	if _, err = CopyLogFileUpdates(dir, dest, sizes); err != nil {
		return nil, nil, err
	}

	if updates, err = testutil.ReadFiles(dest); err != nil {
		return nil, nil, err
	}

	paths = make(map[string]struct{})
	err = filepath.Walk(dest, func(path string, info os.FileInfo, err error) error {
		if err == nil && path != dest {
			paths[path[len(dest)+1:]] = struct{}{}
		}
		return err
	})
	return updates, paths, err
}

func TestCopyUpdates(t *testing.T) {
	sd := testutil.TempDir(t)
	defer os.RemoveAll(sd)

	orig := map[string]string{
		"vegetables":           "kale\ncauliflower\n",
		"baked_goods/desserts": "cake\n",
		"baked_goods/breads":   "",
	}
	if err := testutil.WriteFiles(sd, orig); err != nil {
		t.Fatal(err)
	}

	sizes, _, err := GetLogInodeSizes(sd)
	if err != nil {
		t.Fatal(err)
	}

	updates, _, err := getUpdates(sd, sizes)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 0 {
		t.Errorf("getUpdates(%q, %v) = %v; want none", sd, sizes, updates)
	}

	if err = testutil.AppendToFile(filepath.Join(sd, "vegetables"), "eggplant\n"); err != nil {
		t.Fatal(err)
	}

	// Append to "baked_goods/breads", but then rename it and create a new file with different content.
	if err = testutil.AppendToFile(filepath.Join(sd, "baked_goods/breads"), "ciabatta\n"); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(filepath.Join(sd, "baked_goods/breads"), filepath.Join(sd, "baked_goods/breads.old")); err != nil {
		t.Fatal(err)
	}
	if err = testutil.WriteFiles(sd, map[string]string{"baked_goods/breads": "sourdough\n"}); err != nil {
		t.Fatal(err)
	}

	// Create an empty dir and symlink, neither of which should be copied.
	const (
		emptyDirName = "empty"
		symlinkName  = "veggies"
	)
	if err = os.Mkdir(filepath.Join(sd, emptyDirName), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink("vegetables", filepath.Join(sd, symlinkName)); err != nil {
		t.Fatal(err)
	}

	exp := map[string]string{
		"vegetables":             "eggplant\n",
		"baked_goods/breads.old": "ciabatta\n",
		"baked_goods/breads":     "sourdough\n",
	}
	updates, paths, err := getUpdates(sd, sizes)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updates, exp) {
		t.Errorf("getUpdates(%q, %v) = %v; want %v", sd, sizes, updates, exp)
	}
	for _, p := range []string{emptyDirName, symlinkName} {
		if _, ok := paths[p]; ok {
			t.Errorf("Unwanted path %q was copied", p)
		}
	}
}
