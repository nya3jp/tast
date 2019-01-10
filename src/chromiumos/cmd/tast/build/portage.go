// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package build

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"chromiumos/cmd/tast/timing"
)

const (
	portageConfigFile   = "/etc/make.conf"                 // shell script containing Portage overlays in $PORTDIR_OVERLAY
	portagePackagesFile = "/var/lib/portage/pkgs/Packages" // Portage packages DB file
)

// Matches lines containing first-level dependencies printed by an
// "equery -q -C g --depth=1 <pkg>" command, which produces output
// similar to the following (preceded by a blank line):
//
//	chromeos-base/tast-local-tests-9999:
//	 [  0]  chromeos-base/tast-local-tests-9999
//	 [  1]  chromeos-base/tast-common-9999
//	 [  1]  dev-go/cdp-0.9.1
//	 [  1]  dev-go/dbus-0.0.2-r5
//	 [  1]  dev-lang/go-1.8.3-r1
//	 [  1]  dev-vcs/git-2.12.2
var equeryDepsRegexp = regexp.MustCompile("^\\s*\\[\\s*1\\]\\s+([\\S]+)")

// depInfo contains information about one of a package's dependencies.
type depInfo struct {
	pkg       string // dependency's package name
	installed bool   // true if dependency is installed
	err       error  // non-nil if error encountered while getting status
}

// checkDeps checks if all of portagePkg's direct dependencies are installed.
// Missing packages are returned in the format "<category>/<package>-<version>".
// err is set if a more-serious error is encountered while trying to check dependencies.
func checkDeps(ctx context.Context, portagePkg, cachePath string) (missing []string, err error) {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("check_deps")
		defer st.End()
	}

	// To avoid slow Portage commands, check if we've already verified that dependencies are up-to-date.
	var cache *checkDepsCache
	var lastMod time.Time
	if cachePath != "" {
		checkPaths, err := getOverlays(ctx, portageConfigFile)
		if err != nil {
			return nil, fmt.Errorf("failed to get overlays from %v: %v", portageConfigFile, err)
		}
		checkPaths = append(checkPaths, portagePackagesFile)
		if cache, err = newCheckDepsCache(cachePath, checkPaths); err != nil {
			return nil, fmt.Errorf("failed to load check-deps cache from %v: %v", cachePath, err)
		}
		var checkNeeded bool
		if checkNeeded, lastMod = cache.isCheckNeeded(ctx, portagePkg); !checkNeeded {
			return nil, nil
		}
	}

	// Fall back to the slow equery path.
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("equery")
		defer st.End()
	}

	cmd := exec.Command("equery", "-q", "-C", "g", "--depth=1", portagePkg)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%q failed: %v", strings.Join(cmd.Args, " "), err)
	}

	deps := parseEqueryDeps(out)
	if len(deps) == 0 {
		return nil, fmt.Errorf("no deps found in output from %q", strings.Join(cmd.Args, " "))
	}

	// "equery l" doesn't appear to accept multiple package names, so run queries in parallel.
	ch := make(chan *depInfo, len(deps))
	for _, dep := range deps {
		go func(pkg string) {
			info := &depInfo{pkg: pkg}
			info.installed, info.err = portagePkgInstalled(pkg)
			ch <- info
		}(dep)
	}

	missing = make([]string, 0)
	for range deps {
		info := <-ch
		if info.err != nil {
			return missing, fmt.Errorf("failed getting status of %s: %v", info.pkg, info.err)
		} else if !info.installed {
			missing = append(missing, info.pkg)
		}
	}

	if len(missing) == 0 && cache != nil {
		// Record that dependencies are up-to-date so we can skip these checks next time.
		if err := cache.update(portagePkg, lastMod); err != nil {
			return nil, err
		}
	}

	return missing, nil
}

// parseEqueryDeps parses the output of checkDeps's "equery g" command and returns
// the names (as "<category>/<package>-<version>") of first-level dependencies.
func parseEqueryDeps(out []byte) []string {
	deps := make([]string, 0)
	for _, ln := range strings.Split(string(out), "\n") {
		if matches := equeryDepsRegexp.FindStringSubmatch(ln); matches != nil {
			deps = append(deps, matches[1])
		}
	}
	return deps
}

// portagePkgInstalled runs "equery l" to check if pkg is installed.
func portagePkgInstalled(pkg string) (bool, error) {
	cmd := exec.Command("equery", "-q", "-C", "l", pkg)
	out, err := cmd.Output()
	if err != nil {
		// equery (in "quiet mode") exits with 3 if the package isn't installed.
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				if status.ExitStatus() == 3 {
					return false, nil
				}
			}
		}
		return false, fmt.Errorf("%q failed: %v", strings.Join(cmd.Args, " "), err)
	}

	// equery should print the package name.
	if str := strings.TrimSpace(string(out)); str != pkg {
		return false, fmt.Errorf("%q returned %q", strings.Join(cmd.Args, " "), str)
	}
	return true, nil
}

// getOverlays evaluates the Portage config script at confPath (typically "/etc/make.conf") and returns all of
// the overlays listed in $PORTDIR_OVERLAY. Symlinks are resolved.
func getOverlays(ctx context.Context, confPath string) ([]string, error) {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("get_overlays")
		defer st.End()
	}

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
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("check_cache")
		defer st.End()
	}

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
