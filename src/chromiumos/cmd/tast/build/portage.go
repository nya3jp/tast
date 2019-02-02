// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package build

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/tast/timing"
)

const (
	portageConfigFile  = "/etc/make.conf" // shell script containing Portage overlays in $PORTDIR_OVERLAY
	portagePackagesDir = "/var/db/pkg"    // Portage packages DB dir
)

// checkDeps checks if all of portagePkg's dependencies are installed.
// portagePkg should be a versioned package of the form "chromeos-base/tast-local-tests-cros-9999".
// Missing packages (using the same format) and a list of commands that the user should execute to install
// the dependencies are returned.
func checkDeps(ctx context.Context, portagePkg, cachePath string) (missing, cmds []string, err error) {
	defer timing.Start(ctx, "check_deps").End()

	// To avoid slow Portage commands, check if we've already verified that dependencies are up-to-date.
	var cache *checkDepsCache
	var lastMod time.Time
	if cachePath != "" {
		checkPaths, err := getOverlays(ctx, portageConfigFile)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get overlays from %v: %v", portageConfigFile, err)
		}
		checkPaths = append(checkPaths, portagePackagesDir)
		if cache, err = newCheckDepsCache(cachePath, checkPaths); err != nil {
			return nil, nil, fmt.Errorf("failed to load check-deps cache from %v: %v", cachePath, err)
		}
		var checkNeeded bool
		if checkNeeded, lastMod = cache.isCheckNeeded(ctx, portagePkg); !checkNeeded {
			return nil, nil, nil
		}
	}

	// Fall back to the slow emerge path.
	defer timing.Start(ctx, "emerge").End()

	cl := emergeCmdLine(portagePkg, emergeList)
	cmd := exec.CommandContext(ctx, cl[0], cl[1:]...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	// emerge exits with 1 if the package is masked, so parse the output first to determine
	// whether the error is expected or not.
	missing, masked := parseEmergeOutput(stdout, stderr.Bytes(), portagePkg)
	if err != nil && !masked {
		return nil, nil, fmt.Errorf("%q failed: %v", strings.Join(cl, " "), err)
	}

	if len(missing) == 0 && cache != nil {
		// Record that dependencies are up-to-date so we can skip these checks next time.
		if err := cache.update(portagePkg, lastMod); err != nil {
			return nil, nil, err
		}
	}

	if len(missing) > 0 {
		if masked {
			cmds = append(cmds, fmt.Sprintf("cros_workon --host start '%s'", portagePkg))
		}
		// TODO(derat): Escape args when that's easy to do.
		cmds = append(cmds, strings.Join(emergeCmdLine(portagePkg, emergeInstall), " "))
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
// build dependencies for pkg. pkg should be a versioned package of the form
// "chromeos-base/tast-local-tests-cros-9999".
func emergeCmdLine(pkg string, mode emergeMode) []string {
	var args []string
	add := func(as ...string) { args = append(args, as...) }

	if mode == emergeInstall {
		add("sudo")
	}
	add("emerge", "--jobs=16", "--onlydeps", "--onlydeps-with-rdeps=n", "--update", "--deep", "1")
	if mode == emergeList {
		add("--pretend", "--columns", "--quiet", "y", "--color", "n")
	}
	add("=" + pkg)
	return args
}

// parseEmergeOutput parses stdout and stderr from a command returned by emergeCmdLine and
// returns missing dependencies (as e.g. "dev-go/mdns-0.0.1") and whether a cros_workon command is
// needed to unmask the test bundle before the dependencies can be installed.
//
// emerge prints each missing dependency to stdout on a line similar to the following:
//
//   N     dev-go/cmp 0.2.0-r1
//
// If there are any missing dependencies and the bundle package is masked, then a message
// similar to the following is printed to stderr:
//
//  The following keyword changes are necessary to proceed:
//  (see "package.accept_keywords" in the portage(5) man page for more details)
//  # required by =chromeos-base/tast-local-tests-cros-9999 (argument)
//  =chromeos-base/tast-local-tests-cros-9999 **
//
// Unmasking the package appears to be required even when only emerging its dependencies.
func parseEmergeOutput(stdout, stderr []byte, pkg string) (missingDeps []string, masked bool) {
	for _, ln := range strings.Split(strings.TrimSpace(string(stdout)), "\n") {
		fields := strings.Fields(ln)
		if len(fields) < 3 || strings.Index(fields[1], "/") == -1 {
			continue
		}
		missingDeps = append(missingDeps, fields[1]+"-"+fields[2])
	}

	maskLine := fmt.Sprintf("=%s **", pkg)
	for _, ln := range strings.Split(string(stderr), "\n") {
		if strings.TrimSpace(ln) == maskLine {
			masked = true
		}
	}

	return missingDeps, masked
}

// getOverlays evaluates the Portage config script at confPath (typically "/etc/make.conf") and returns all of
// the overlays listed in $PORTDIR_OVERLAY. Symlinks are resolved.
func getOverlays(ctx context.Context, confPath string) ([]string, error) {
	defer timing.Start(ctx, "get_overlays").End()

	// TODO(derat): Escape the args when we have a good way to do so (testexec is only available to tests).
	if strings.Index(confPath, "'") != -1 {
		return nil, errors.New("single quotes unsupported")
	}
	shCmd := fmt.Sprintf("cd '%s' && source '%s' && echo $PORTDIR_OVERLAY", filepath.Dir(confPath), filepath.Base(confPath))
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
	cachePath  string               // path to JSON file with map from package name to last-modified timestamp
	pkgLastMod map[string]time.Time // contents of cachePath
	checkPaths []string             // Portage overlay dirs and package file that should be checked for modifications
}

// newCheckDepsCache reads and unmarshals cachePath and returns a new checkDepsCache.
// No error is returned if cachePath doesn't already exist.
func newCheckDepsCache(cachePath string, checkPaths []string) (*checkDepsCache, error) {
	c := &checkDepsCache{
		cachePath:  cachePath,
		pkgLastMod: make(map[string]time.Time),
		checkPaths: checkPaths,
	}

	f, err := os.Open(cachePath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	} else if err == nil {
		defer f.Close()
		if err := json.NewDecoder(f).Decode(&c.pkgLastMod); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// isCheckNeeded compares the current state of the filesystem against the last time that dependencies were verified as
// being up-to-date for pkg. checkNeeded is true if the two timestamps do not exactly match. The filesystem's latest
// last-modified timestamps is returned and should be passed to update if the dependencies are up-to-date.
func (c *checkDepsCache) isCheckNeeded(ctx context.Context, pkg string) (checkNeeded bool, lastMod time.Time) {
	defer timing.Start(ctx, "check_cache").End()

	ch := make(chan time.Time, len(c.checkPaths))
	for _, p := range c.checkPaths {
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
	for range c.checkPaths {
		if t := <-ch; t.After(lastMod) {
			lastMod = t
		}
	}

	cachedLastMod := c.pkgLastMod[pkg]
	return cachedLastMod.IsZero() || !cachedLastMod.Equal(lastMod), lastMod
}

// update sets pkg's last-modified timestamp and atomically overwrites the on-disk copy of the cache.
func (c *checkDepsCache) update(pkg string, lastMod time.Time) error {
	c.pkgLastMod[pkg] = lastMod

	if err := os.MkdirAll(filepath.Dir(c.cachePath), 0755); err != nil {
		return err
	}
	f, err := ioutil.TempFile(filepath.Dir(c.cachePath), filepath.Base(c.cachePath)+".")
	if err != nil {
		return err
	}
	if err := json.NewEncoder(f).Encode(&c.pkgLastMod); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), c.cachePath)

}
