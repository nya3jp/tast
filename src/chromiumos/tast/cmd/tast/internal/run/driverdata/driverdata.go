// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package driverdata defines data types for the driver package.
package driverdata

import (
	"chromiumos/tast/internal/protocol"
)

// BundleEntity is a pair of a ResolvedEntity and its bundle name.
// TODO: Move this type to run/driver/internal/driverdata. It is in this package
// just for a reference from config.State.
type BundleEntity struct {
	Bundle   string
	Resolved *protocol.ResolvedEntity
}
