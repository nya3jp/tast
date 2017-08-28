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

	"chromiumos/tast/common/testutil"
)

const (
	tempDirPrefix = "logs_test."
)

// getUpdates passes sizes to CopyLogFileUpdates to get file updates within dir
// and then returns the updates as a map from relative filename to content.
func getUpdates(dir string, sizes InodeSizes) (dest string, updates map[string]string, err error) {
	dest, err = ioutil.TempDir("", tempDirPrefix)
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(dest)

	if _, err = CopyLogFileUpdates(dir, dest, sizes); err != nil {
		return "", nil, err
	}
	updates, err = testutil.ReadFiles(dest)
	return dest, updates, err
}

func TestCopyUpdates(t *testing.T) {
	sd, err := ioutil.TempDir("", tempDirPrefix)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(sd)

	orig := map[string]string{
		"vegetables":           "kale\ncauliflower\n",
		"baked_goods/desserts": "cake\n",
		"baked_goods/breads":   "",
	}
	if err = testutil.WriteFiles(sd, orig); err != nil {
		t.Fatal(err)
	}

	sizes, _, err := GetLogInodeSizes(sd)
	if err != nil {
		t.Fatal(err)
	}

	if _, updates, err := getUpdates(sd, sizes); err != nil {
		t.Fatal(err)
	} else if len(updates) != 0 {
		t.Errorf("getUpdates(%v, %v) = %v; want none", sd, sizes, updates)
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
	dd, updates, err := getUpdates(sd, sizes)
	if err != nil {
		t.Fatal(err)
	} else if !reflect.DeepEqual(updates, exp) {
		t.Errorf("getUpdates(%v, %v) = %v; want %v", sd, sizes, updates, exp)
	}
	for _, p := range []string{emptyDirName, symlinkName} {
		if _, err := os.Stat(filepath.Join(dd, p)); !os.IsNotExist(err) {
			t.Errorf("Unwanted path %q was copied", p)
		}
	}
}
