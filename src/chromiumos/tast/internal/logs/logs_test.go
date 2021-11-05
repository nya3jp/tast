// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logs_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/logging/loggingtest"
	"chromiumos/tast/internal/logs"
	"chromiumos/tast/testutil"
)

// getUpdates passes sizes to CopyLogFileUpdates to get file updates within dir
// and then returns the copied data as a map from relative filename to content,
// along with a set containing all copied relative paths (including empty files and broken symlinks).
func getUpdates(ctx context.Context, dir string, exclude []string, sizes logs.InodeSizes) (
	updates map[string]string, paths map[string]struct{}, err error) {
	dest, err := ioutil.TempDir("", "tast_logs.")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(dest)

	if err := logs.CopyLogFileUpdates(ctx, dir, dest, exclude, sizes); err != nil {
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
	logger := loggingtest.NewLogger(t, logging.LevelDebug)
	ctx := logging.AttachLogger(context.Background(), logger)

	sd := testutil.TempDir(t)
	defer os.RemoveAll(sd)

	orig := map[string]string{
		"vegetables":               "kale\ncauliflower\n",
		"fish":                     "salmon\ntuna\n",
		"baked_goods/desserts":     "cake\n",
		"baked_goods/breads":       "",
		"baked_goods/stale_crumbs": "two days old\n",
		"toppings/sauces":          "tomato\n",
	}
	if err := testutil.WriteFiles(sd, orig); err != nil {
		t.Fatal(err)
	}

	exclude := []string{
		"baked_goods/stale_crumbs",
		"baked_goods/fresh_crumbs",
		"toppings",
	}
	sizes, err := logs.GetLogInodeSizes(ctx, sd, exclude)
	if err != nil {
		t.Fatal(err)
	}

	updates, _, err := getUpdates(ctx, sd, exclude, sizes)
	if err != nil {
		t.Fatal(err)
	}
	if len(logger.Logs()) != 0 {
		t.Errorf("getUpdates(%q, %v) emitted warnings", sd, sizes)
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

	// Append to existing files that should be skipped, and create new files that should also be skipped.
	if err = testutil.AppendToFile(filepath.Join(sd, "baked_goods/stale_crumbs"), "one day old\n"); err != nil {
		t.Fatal(err)
	}
	if err = testutil.AppendToFile(filepath.Join(sd, "toppings/sauces"), "alfredo\n"); err != nil {
		t.Fatal(err)
	}
	if err = testutil.WriteFiles(sd, map[string]string{
		"baked_goods/fresh_crumbs": "just out of the oven\n",
		"toppings/crumbles":        "blue cheese\n",
	}); err != nil {
		t.Fatal(err)
	}

	// Truncate existing file. Should be detected for a warning.
	if err = testutil.WriteFiles(sd, map[string]string{
		"fish": "salmon\n",
	}); err != nil {
		t.Fatal(err)
	}

	exp := map[string]string{
		"vegetables":             "eggplant\n",
		"fish":                   "salmon\n",
		"baked_goods/breads.old": "ciabatta\n",
		"baked_goods/breads":     "sourdough\n",
	}
	updates, paths, err := getUpdates(ctx, sd, exclude, sizes)
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

	// Expect 1 warning.
	got := logger.Logs()
	want := []string{
		fmt.Sprintf(
			"%s is shorter than original (now 7, original 12), copying all instead of diff",
			filepath.Join(sd, "fish")),
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("getUpdates(%q, %v) emitted unexpected warnings (-got +want):\n%s", sd, sizes, diff)
	}
}
