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

func TestParseEqueryDeps(t *testing.T) {
	// Copy-and-pasted output (including trailing whitespace) from
	// "equery -q -C g --depth=1 chromeos-base/tast-local-tests-9999".
	out := `
chromeos-base/tast-local-tests-9999:
 [  0]  chromeos-base/tast-local-tests-9999   
 [  1]  chromeos-base/tast-common-9999   
 [  1]  dev-go/cdp-0.9.1   
 [  1]  dev-go/dbus-0.0.2-r5   
 [  1]  dev-lang/go-1.8.3-r1   
 [  1]  dev-vcs/git-2.12.2   
`

	exp := []string{
		"chromeos-base/tast-common-9999",
		"dev-go/cdp-0.9.1",
		"dev-go/dbus-0.0.2-r5",
		"dev-lang/go-1.8.3-r1",
		"dev-vcs/git-2.12.2",
	}
	if act := parseEqueryDeps([]byte(out)); !reflect.DeepEqual(act, exp) {
		t.Errorf("parseEqueryDeps(%q) = %v; want %v", out, act, exp)
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

		pkg = "chromeos-base/mypkg-9999"
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
		checkNeeded, lastMod = cache.isCheckNeeded(context.Background(), pkg)
		return cache, checkNeeded, lastMod
	}

	// The cache should initially report that dependencies need to be checked.
	cache, checkNeeded, lastMod := createCache()
	if !checkNeeded {
		t.Errorf("isCheckNeeded(%q) is false initially", pkg)
	} else if !lastMod.Equal(t0) {
		t.Errorf("isCheckNeeded(%q) returned last mod %v initially; want %v", pkg, lastMod, t0)
	}

	// After writing the updated timestamp to the cache, no check is needed.
	if err := cache.update(pkg, lastMod); err != nil {
		t.Fatalf("update(%q, %v) failed: ", pkg, lastMod)
	}
	if cache, checkNeeded, lastMod = createCache(); checkNeeded {
		t.Errorf("isCheckNeeded(%q) is true after update", pkg)
	}

	// After files in overlays are updated, checking is needed again.
	setTimes(filepath.Join(overlay1Path, file1), t2)
	setTimes(filepath.Join(overlay2Path, file2), t1)
	if cache, checkNeeded, lastMod = createCache(); !checkNeeded {
		t.Errorf("isCheckNeeded(%q) is false after overlay update", pkg)
	} else if !lastMod.Equal(t2) {
		t.Errorf("isCheckNeeded(%q) returned last mod %v after overlay update; want %v", pkg, lastMod, t2)
	}

	// Updating the cache should make checking unnecessary again.
	if err := cache.update(pkg, lastMod); err != nil {
		t.Fatalf("update(%q, %v) failed: ", pkg, lastMod)
	}
	if cache, checkNeeded, lastMod = createCache(); checkNeeded {
		t.Errorf("isCheckNeeded(%q) is true after second update", pkg)
	}

	// Updating the DB file should also result in a check being needed.
	setTimes(dbPath, t3)
	if cache, checkNeeded, lastMod = createCache(); !checkNeeded {
		t.Errorf("isCheckNeeded(%q) is false after DB update", pkg)
	} else if !lastMod.Equal(t3) {
		t.Errorf("isCheckNeeded(%q) returned last mod %v after DB update; want %v", pkg, lastMod, t3)
	}
}
