// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"chromiumos/tast/autocaps"
	"chromiumos/tast/command"
	"chromiumos/tast/expr"
)

const autotestCapPrefix = "autotest-capability:" // prefix for autotest-capability feature names

// handleGetSoftwareFeatures handles a GetSoftwareFeaturesMode request from args
// and JSON-marshals a GetSoftwareFeaturesResult struct to w.
func handleGetSoftwareFeatures(args *Args, w io.Writer) error {
	if args.USEFlagsFile == "" {
		return command.NewStatusErrorf(statusBadArgs, "feature enumeration unsupported")
	}

	// If the file listing USE flags doesn't exist, we're probably running on a non-test
	// image. Return an empty response to signal that to the caller.
	if _, err := os.Stat(args.USEFlagsFile); os.IsNotExist(err) {
		return json.NewEncoder(w).Encode(&GetSoftwareFeaturesResult{})
	}
	flags, err := readUSEFlagsFile(args.USEFlagsFile)
	if err != nil {
		return err
	}

	res := GetSoftwareFeaturesResult{}
	var autotestCaps map[string]autocaps.State
	if args.AutotestCapabilityDir != "" {
		if ac, err := autocaps.Read(args.AutotestCapabilityDir, nil); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %v", args.AutotestCapabilityDir, err))
		} else {
			autotestCaps = ac
		}
	}

	if res.Available, res.Unavailable, err =
		determineSoftwareFeatures(args.SoftwareFeatureDefinitions, flags, autotestCaps); err != nil {
		return command.NewStatusErrorf(statusError, "%s", err)
	}
	return json.NewEncoder(w).Encode(&res)
}

// readUSEFlagsFile reads a list of USE flags from fn (see Args.USEFlagsFile).
// Each flag should be specified on its own line, and lines beginning with '#' are ignored.
func readUSEFlagsFile(fn string) ([]string, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var flags []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		flag := strings.TrimSpace(sc.Text())
		if flag != "" && flag[0] != '#' {
			flags = append(flags, flag)
		}
	}
	if err = sc.Err(); err != nil {
		return nil, command.NewStatusErrorf(statusError, "failed to read %v: %v", fn, err)
	}
	return flags, err
}

// determineSoftwareFeatures computes the DUT's available and unavailable software features.
// definitions maps feature names to definitions (see Args.SoftwareFeatureDefinitions).
// useFlags contains a list of relevant USE flags that were set when building the system image (see Args.USEFlagsFile).
// autotestCaps contains a mapping from autotest-capability names to the corresponding states.
func determineSoftwareFeatures(definitions map[string]string, useFlags []string,
	autotestCaps map[string]autocaps.State) (
	available, unavailable []string, err error) {
	for ft, es := range definitions {
		if strings.HasPrefix(ft, autotestCapPrefix) {
			return nil, nil, fmt.Errorf("feature %q has reserved prefix %q", ft, autotestCapPrefix)
		}

		ex, err := expr.New(es)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse %q feature expression %q: %v", ft, es, err)
		}
		if ex.Matches(useFlags) {
			available = append(available, ft)
		} else {
			unavailable = append(unavailable, ft)
		}
	}

	for name, state := range autotestCaps {
		if state == autocaps.Yes {
			available = append(available, autotestCapPrefix+name)
		} else {
			unavailable = append(unavailable, autotestCapPrefix+name)
		}
	}

	sort.Strings(available)
	sort.Strings(unavailable)
	return available, unavailable, nil
}
