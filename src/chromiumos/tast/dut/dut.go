// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dut provides a connection to a DUT ("Device Under Test")
// for use by remote tests.
package dut

import (
	"context"

	"go.chromium.org/tast/core/dut"
)

// DUT represents a "Device Under Test" against which remote tests are run.
type DUT = dut.DUT

// New returns a new DUT usable for communication with target
// (of the form "[<user>@]host[:<port>]") using the SSH key at keyFile or
// keys located in keyDir.
// The DUT does not start out in a connected state; Connect must be called.
func New(target, keyFile, keyDir, proxyCommand string, beforeReboot func(context.Context, *DUT) error) (*DUT, error) {
	return dut.New(target, keyFile, keyDir, proxyCommand, beforeReboot)
}
