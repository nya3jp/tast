// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package drivercore defines core data types for the driver package.
package drivercore

import (
	"go.chromium.org/tast/core/internal/protocol"
)

// BundleEntity is a pair of a ResolvedEntity and its bundle name.
type BundleEntity struct {
	Bundle   string
	Resolved *protocol.ResolvedEntity
}
