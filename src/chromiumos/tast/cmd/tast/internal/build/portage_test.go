// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package build

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"chromiumos/tast/testutil"
)

func TestParseEmergeOutput(t *testing.T) {
	const (
		name1 = "chromeos-base/my_pkg"
		ver1  = "0.0.1-r3"
		dep1  = name1 + "-" + ver1

		name2 = "dev-go/some-lib"
		ver2  = "0.0.1-r21"
		dep2  = name2 + "-" + ver2
	)

	for _, tc := range []struct {
		stdout  string
		missing []string
	}{
		{"", nil},
		{fmt.Sprintf(" N     %s %s\n", name1, ver1), []string{dep1}},
		{fmt.Sprintf(" N     %s %s\n U     %s %s\n", name1, ver1, name2, ver2), []string{dep1, dep2}},
		{fmt.Sprintf(" N     %s %s\n", name1, ver1), []string{dep1}},
	} {
		if missing := parseMissingDeps([]byte(tc.stdout)); !reflect.DeepEqual(missing, tc.missing) {
			t.Errorf("parseMissingDeps(%q) = %v; want %v", tc.stdout, missing, tc.missing)
		}
	}
}

func TestGetOverlays(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	// Regular directories should be returned.
	overlay := filepath.Join(td, "overlay")
	if err := os.Mkdir(overlay, 0755); err != nil {
		t.Fatal("Failed creating dir: ", err)
	}

	// Symlinks to directories should be followed.
	target := filepath.Join(td, "target")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal("Failed creating dir: ", err)
	}
	link := filepath.Join(td, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal("Failed creating symlink: ", err)
	}

	// Broken symlinks and regular files should be skipped.
	broken := filepath.Join(td, "broken")
	if err := os.Symlink("bogus", broken); err != nil {
		t.Fatal("Failed creating symlink: ", err)
	}
	file := filepath.Join(td, "file")
	if err := ioutil.WriteFile(file, []byte{}, 0644); err != nil {
		t.Fatal("Failed writing file: ", err)
	}

	conf := filepath.Join(td, "make.conf")
	data := fmt.Sprintf(`PORTDIR_OVERLAY="%s"`, strings.Join([]string{overlay, link, broken, file}, " "))
	if err := ioutil.WriteFile(conf, []byte(data), 0644); err != nil {
		t.Fatal("Failed writing config: ", err)
	}

	overlays, err := getOverlays(context.Background(), conf)
	if err != nil {
		t.Fatalf("getOverlays(%q) failed: %v", conf, err)
	}
	sort.Strings(overlays)
	if exp := []string{overlay, target}; !reflect.DeepEqual(overlays, exp) {
		t.Errorf("getOverlays(%q) = %v; want %v", conf, overlays, exp)
	}
}

func TestCheckDepsCache(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	const (
		overlay1 = "overlay1" // in td
		overlay2 = "overlay2" // in td
		file1    = "foo.txt"  // in overlay1
		file2    = "bar.txt"  // in overlay2
		dbFile   = "packages" // in td
	)

	if err := testutil.WriteFiles(td, map[string]string{
		filepath.Join(overlay1, file1): "foo",
		filepath.Join(overlay2, file2): "bar",
		dbFile:                         "db",
	}); err != nil {
		t.Fatal("Failed writing files: ", err)
	}

	// setTimes sets the atime and mtime on root and its contents to tm.
	setTimes := func(root string, tm time.Time) {
		filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
			if err != nil {
				t.Fatalf("Failed to walk %v: %v", p, err)
			}
			if err := os.Chtimes(p, tm, tm); err != nil {
				t.Fatalf("Failed to set times on %v: %v", p, err)
			}
			return nil
		})
	}

	t0 := time.Unix(0, 0)
	t1 := t0.Add(time.Second)
	t2 := t0.Add(time.Minute)
	t3 := t0.Add(time.Hour)

	overlay1Path := filepath.Join(td, overlay1)
	overlay2Path := filepath.Join(td, overlay2)
	dbPath := filepath.Join(td, dbFile)

	// All files initially have the same timestamp.
	setTimes(overlay1Path, t0)
	setTimes(overlay2Path, t0)
	setTimes(dbPath, t0)

	cachePath := filepath.Join(td, "build/cache.json")

	// createCache calls newCheckDepsCache and isCheckNeeded and returns the resulting values.
	createCache := func() (cache *checkDepsCache, checkNeeded bool, lastMod time.Time) {
		cache, err := newCheckDepsCache(cachePath, []string{dbPath, overlay1Path, overlay2Path})
		if err != nil {
			t.Fatalf("Failed to open %v: %v", cachePath, err)
		}
		checkNeeded, lastMod = cache.isCheckNeeded(context.Background())
		return cache, checkNeeded, lastMod
	}

	// The cache should initially report that dependencies need to be checked.
	cache, checkNeeded, lastMod := createCache()
	if !checkNeeded {
		t.Error("isCheckNeeded is false initially")
	} else if !lastMod.Equal(t0) {
		t.Errorf("isCheckNeeded returned last mod %v initially; want %v", lastMod, t0)
	}

	// After writing the updated timestamp to the cache, no check is needed.
	if err := cache.update(cachePath, lastMod); err != nil {
		t.Fatalf("update(%v) failed: ", lastMod)
	}
	if cache, checkNeeded, lastMod = createCache(); checkNeeded {
		t.Errorf("isCheckNeeded is true after update")
	}

	// After files in overlays are updated, checking is needed again.
	setTimes(filepath.Join(overlay1Path, file1), t2)
	setTimes(filepath.Join(overlay2Path, file2), t1)
	if cache, checkNeeded, lastMod = createCache(); !checkNeeded {
		t.Error("isCheckNeeded is false after overlay update")
	} else if !lastMod.Equal(t2) {
		t.Errorf("isCheckNeeded returned last mod %v after overlay update; want %v", lastMod, t2)
	}

	// Updating the cache should make checking unnecessary again.
	if err := cache.update(cachePath, lastMod); err != nil {
		t.Fatalf("update(%v) failed: ", lastMod)
	}
	if cache, checkNeeded, lastMod = createCache(); checkNeeded {
		t.Error("isCheckNeeded is true after second update")
	}

	// Updating the DB file should also result in a check being needed.
	setTimes(dbPath, t3)
	if cache, checkNeeded, lastMod = createCache(); !checkNeeded {
		t.Error("isCheckNeeded is false after DB update")
	} else if !lastMod.Equal(t3) {
		t.Errorf("isCheckNeeded returned last mod %v after DB update; want %v", lastMod, t3)
	}
}
