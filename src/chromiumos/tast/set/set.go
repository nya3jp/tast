// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package set provides utility set operations.
package set

// StringSliceDiff returns cur - orig (where - is  the set difference operator).
// In other words, it returns all elements of |cur| that are not in |orig|.
func StringSliceDiff(orig, cur []string) (added []string) {
	om := make(map[string]bool, len(orig))
	for _, p := range orig {
		om[p] = true
	}

	for _, p := range cur {
		if !om[p] {
			added = append(added, p)
		}
	}
	return added
}
