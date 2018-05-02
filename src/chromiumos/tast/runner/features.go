// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"sort"
	"strings"

	"chromiumos/tast/command"
	"chromiumos/tast/expr"
)

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
	for ft, es := range args.SoftwareFeatureDefinitions {
		ex, err := expr.New(es)
		if err != nil {
			return command.NewStatusErrorf(statusError, "failed to parse feature expression %q: %v", es, err)
		}
		if ex.Matches(flags) {
			res.Available = append(res.Available, ft)
		} else {
			res.Unavailable = append(res.Unavailable, ft)
		}
	}
	sort.Strings(res.Available)
	sort.Strings(res.Unavailable)
	return json.NewEncoder(w).Encode(&res)
}

// readUSEFlagsFile reads a list of USE flags from fn.
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
