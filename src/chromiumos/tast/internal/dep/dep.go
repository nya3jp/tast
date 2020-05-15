// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dep deals with software/hardware dependencies of tests.
package dep

// Features contains information about all features of the DUT.
type Features struct {
	// Software contains information about software features.
	// If it is nil, software dependency checks should not be performed.
	Software *SoftwareFeatures

	// Hardware contains information about hardware features.
	// If it is nil, hardware dependency checks should not be performed.
	Hardware *HardwareFeatures
}
