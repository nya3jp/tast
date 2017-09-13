// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package build

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"syscall"

	"chromiumos/tast/tast/timing"
)

var equeryDepsRegexp *regexp.Regexp

func init() {
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
	equeryDepsRegexp = regexp.MustCompile("^\\s*\\[\\s*1\\]\\s+([\\S]+)")
}

// depInfo contains information about one of a package's dependencies.
type depInfo struct {
	pkg       string // dependency's package name
	installed bool   // true if dependency is installed
	err       error  // non-nil if error encountered while getting status
}

// checkDeps checks if all of portagePkg's direct dependencies are installed.
// Missing dependencies are returned in badDeps, with package names in
// the format "<category>/<package>-<version>" as keys and descriptive error
// messages as values. err is set if a more-serious error is encountered while
// trying to check dependencies.
func checkDeps(ctx context.Context, portagePkg string) (missing []string, err error) {
	if tl, ok := timing.FromContext(ctx); ok {
		st := tl.Start("check_deps")
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
