// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dep

import (
	"sort"

	"chromiumos/tast/internal/protocol"
)

// SoftwareDeps represents dependencies to software features.
type SoftwareDeps = []string

// missingSoftwareDeps returns a sorted list of dependencies from SoftwareDeps
// that aren't present on the DUT (per the passed-in features list).
// unknown is a sorted list of unknown software features. It is always a subset
// of missing.
func missingSoftwareDeps(deps SoftwareDeps, features *protocol.SoftwareFeatures) (missing, unknown []string) {
DepLoop:
	for _, d := range deps {
		for _, f := range features.GetAvailable() {
			if d == f {
				continue DepLoop
			}
		}
		missing = append(missing, d)
		for _, f := range features.GetUnavailable() {
			if d == f {
				continue DepLoop
			}
		}
		unknown = append(unknown, d)
	}
	sort.Strings(missing)
	sort.Strings(unknown)
	return missing, unknown
}
