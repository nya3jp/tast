// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crosbundle

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"chromiumos/tast/autocaps"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/expr"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/lsbrelease"
)

const autotestCapPrefix = "autotest-capability:" // prefix for autotest-capability feature names

// DetectSoftwareFeatures implements the main function of RunnerGetDUTInfoMode (i.e., except input/output
// conversion for RPC).
func DetectSoftwareFeatures(ctx context.Context, definitions map[string]string, useFlagsFile, lsbReleaseFile string, extraUSEFlags []string, autotestCapsDir string) (*protocol.SoftwareFeatures, error) {
	if useFlagsFile == "" {
		return nil, errors.New("feature enumeration unsupported")
	}

	// If the file listing USE flags doesn't exist, we're probably running on a non-test
	// image. Return an empty response to signal that to the caller.
	if _, err := os.Stat(useFlagsFile); os.IsNotExist(err) {
		return nil, nil
	}

	flags, err := readUSEFlagsFile(useFlagsFile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %v", useFlagsFile)
	}
	flags = append(flags, extraUSEFlags...)

	if lsbReleaseFile == "" {
		logging.Info(ctx, "lsb-release path is not specified; board names in software feature definitions will not work")
	} else if lr, err := lsbrelease.LoadFrom(lsbReleaseFile); err != nil {
		logging.Infof(ctx, "Failed to read lsbrelease; board names in software feature definitions will not work: %v", err)
	} else if board, ok := lr[lsbrelease.Board]; !ok {
		logging.Infof(ctx, "Failed to find boardname in lsbrelease; board names in software feature definitions will not work")
	} else {
		flags = append(flags, "board:"+board)
	}

	var autotestCaps map[string]autocaps.State
	if autotestCapsDir != "" {
		if ac, err := autocaps.Read(autotestCapsDir, nil); err != nil {
			logging.Infof(ctx, "%s: %v", autotestCapsDir, err)
		} else {
			autotestCaps = ac
		}
	}

	features, err := determineSoftwareFeatures(definitions, flags, autotestCaps)
	if err != nil {
		return nil, err
	}
	return features, nil
}

// readUSEFlagsFile reads a list of USE flags from fn (see StaticConfig.USEFlagsFile).
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
		return nil, err
	}
	return flags, nil
}

// determineSoftwareFeatures computes the DUT's available and unavailable software features.
// definitions maps feature names to definitions (see StaticConfig.SoftwareFeatureDefinitions).
// useFlags contains a list of relevant USE flags that were set when building the system image (see StaticConfig.USEFlagsFile).
// autotestCaps contains a mapping from autotest-capability names to the corresponding states.
func determineSoftwareFeatures(definitions map[string]string, useFlags []string, autotestCaps map[string]autocaps.State) (
	*protocol.SoftwareFeatures, error) {
	var available, unavailable []string
	for ft, es := range definitions {
		if strings.HasPrefix(ft, autotestCapPrefix) {
			return nil, fmt.Errorf("feature %q has reserved prefix %q", ft, autotestCapPrefix)
		}

		ex, err := expr.New(es)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %q feature expression %q: %v", ft, es, err)
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
	return &protocol.SoftwareFeatures{Available: available, Unavailable: unavailable}, nil
}
