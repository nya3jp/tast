// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dep

// SoftwareFeatures contains information about software features of the DUT.
type SoftwareFeatures struct {
	// Available contains a list of software features supported by the DUT.
	Available []string

	// Unavailable contains a list of software features not supported by the DUT.
	Unavailable []string
}
