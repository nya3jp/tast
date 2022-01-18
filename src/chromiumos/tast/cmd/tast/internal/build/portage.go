// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package build

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"chromiumos/tast/internal/timing"
	"chromiumos/tast/shutil"
)

const (
	portageConfigFile  = "/etc/make.conf" // shell script containing Portage overlays in $PORTDIR_OVERLAY
	portagePackagesDir = "/var/db/pkg"    // Portage packages DB dir

	buildDepsPkg = "chromeos-base/tast-build-deps" // Portage package depending on all build dependencies
)

// checkDeps checks if all of build dependencies are installed.
// Missing packages and a list of commands that can be executed to install the dependencies are returned.
func checkDeps(ctx context.Context, cachePath string) (
	missing []string, cmds [][]string, err error) {
	ctx, st1 := timing.Start(ctx, "check_deps")
	defer st1.End()

	// To avoid slow Portage commands, check if we've already verified that dependencies are up-to-date.
	checkPaths, err := getOverlays(ctx, portageConfigFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get overlays from %v: %v", portageConfigFile, err)
	}
	checkPaths = append(checkPaths, portagePackagesDir)
	cache, err := newCheckDepsCache(cachePath, checkPaths)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load check-deps cache from %v: %v", cachePath, err)
	}
	checkNeeded, lastMod := cache.isCheckNeeded(ctx)
	if !checkNeeded {
		return nil, nil, nil
	}

	// Fall back to the slow (multiple seconds) emerge path.
	ctx, st2 := timing.Start(ctx, "emerge_list_deps")
	defer st2.End()

	cl := emergeCmdLine(emergeList)
	stdout, err := exec.CommandContext(ctx, cl[0], cl[1:]...).Output()
	if err != nil {
		return nil, nil, fmt.Errorf("%q failed: %v", shutil.EscapeSlice(cl), err)
	}

	missing = parseMissingDeps(stdout)
	if len(missing) == 0 {
		// Record that dependencies are up-to-date so we can skip these checks next time.
		if err := cache.update(cachePath, lastMod); err != nil {
			return nil, nil, err
		}
	} else {
		cmds = append(cmds, emergeCmdLine(emergeInstall))
	}

	return missing, cmds, nil
}

// emergeMode describes a mode to use when running emerge.
type emergeMode int

const (
	emergeList    emergeMode = iota // list missing dependencies
	emergeInstall                   // install missing dependencies
)

// emergeCmdLine returns an emerge command that lists or installs missing or outdated
// build dependencies for buildDepsPkg.
func emergeCmdLine(mode emergeMode) []string {
	var args []string
	add := func(as ...string) { args = append(args, as...) }

	if mode == emergeInstall {
		add("sudo")
	}
	add("emerge", "--jobs=16", "--usepkg", "--onlydeps", "--update", "--deep", "1")
	if mode == emergeList {
		add("--pretend", "--columns", "--quiet", "y", "--color", "n")
	}
	add(buildDepsPkg)
	return args
}

// parseMissingDeps parses stdout and stderr from a command returned by emergeCmdLine and
// returns missing dependencies (as e.g. "dev-go/mdns-0.0.1").
//
// emerge prints each missing dependency to stdout on a line similar to the following:
//
//   N     dev-go/cmp 0.2.0-r1
func parseMissingDeps(stdout []byte) []string {
	var missing []string
	for _, ln := range strings.Split(strings.TrimSpace(string(stdout)), "\n") {
		fields := strings.Fields(ln)
		if len(fields) < 3 || strings.Index(fields[1], "/") == -1 {
			continue
		}
		missing = append(missing, fields[1]+"-"+fields[2])
	}
	return missing
}

// getOverlays evaluates the Portage config script at confPath (typically "/etc/make.conf") and returns all of
// the overlays listed in $PORTDIR_OVERLAY. Symlinks are resolved.
func getOverlays(ctx context.Context, confPath string) ([]string, error) {
	ctx, st := timing.Start(ctx, "get_overlays")
	defer st.End()

	shCmd := fmt.Sprintf("cd %s && source %s && echo $PORTDIR_OVERLAY",
		shutil.Escape(filepath.Dir(confPath)), shutil.Escape(filepath.Base(confPath)))
	out, err := exec.CommandContext(ctx, "bash", "-e", "-c", shCmd).Output()
	if err != nil {
		return nil, err
	}

	var overlays []string
	for _, p := range strings.Fields(string(out)) {
		if p, err = filepath.EvalSymlinks(p); err != nil {
			continue // ignore broken symlinks
		}
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			overlays = append(overlays, p)
		}
	}
	return overlays, nil
}

// checkDepsCache is used to track and check successful calls to checkDeps to make it possible to skip
// executing slow Portage commands when nothing has changed.
type checkDepsCache struct {
	CheckPaths []string  `json:"checkPaths"` // paths to check timestamps in
	LastMod    time.Time `json:"lastMod"`    // latest timestamp of the all files in CheckPaths
}

// newCheckDepsCache reads and unmarshals cachePath and returns a new checkDepsCache.
// No error is returned if cachePath doesn't already exist.
func newCheckDepsCache(cachePath string, checkPaths []string) (*checkDepsCache, error) {
	f, err := os.Open(cachePath)
	if os.IsNotExist(err) {
		return &checkDepsCache{CheckPaths: checkPaths}, nil
	} else if err != nil {
		return nil, err
	}
	defer f.Close()

	var cache checkDepsCache
	if err := json.NewDecoder(f).Decode(&cache); err != nil {
		return nil, err
	}
	// Invalidate the cache if CheckPaths has changed.
	if !reflect.DeepEqual(cache.CheckPaths, checkPaths) {
		return &checkDepsCache{CheckPaths: checkPaths}, nil
	}
	return &cache, nil
}

// isCheckNeeded compares the current state of the filesystem against the last time that dependencies were verified as
// being up-to-date for pkg. checkNeeded is true if the two timestamps do not exactly match. The filesystem's latest
// last-modified timestamps is returned and should be passed to update if the dependencies are up-to-date.
func (c *checkDepsCache) isCheckNeeded(ctx context.Context) (checkNeeded bool, lastMod time.Time) {
	ctx, st := timing.Start(ctx, "check_cache")
	defer st.End()

	ch := make(chan time.Time, len(c.CheckPaths))
	for _, p := range c.CheckPaths {
		go func(p string) {
			var latest time.Time
			filepath.Walk(p, func(_ string, fi os.FileInfo, err error) error {
				if err == nil && fi.ModTime().After(latest) {
					latest = fi.ModTime()
				}
				return nil
			})
			ch <- latest
		}(p)
	}
	for range c.CheckPaths {
		if t := <-ch; t.After(lastMod) {
			lastMod = t
		}
	}

	return c.LastMod.IsZero() || !c.LastMod.Equal(lastMod), lastMod
}

// update atomically overwrites the on-disk copy of the cache.
func (c *checkDepsCache) update(cachePath string, lastMod time.Time) error {
	c.LastMod = lastMod

	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return err
	}
	f, err := ioutil.TempFile(filepath.Dir(cachePath), filepath.Base(cachePath)+".")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := json.NewEncoder(f).Encode(c); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), cachePath)
}
