// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// DO NOT USE THIS COPY OF SERVO IN TESTS, USE THE ONE IN platform/tast-tests/src/chromiumos/tast/common/servo

// Package servo is used to communicate with servo devices connected to DUTs.
// It communicates with servod over XML-RPC.
// More details on servo: https://www.chromium.org/chromium-os/servo
//
// Caution: If you reboot the ChromeOS EC:
// - If using a CCD servo, you should call WatchdogRemove(ctx, CCD) or servod will fail.
// - Use Helper.WaitConnect instead of DUT.WaitConnect.
package servo

import (
	"context"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/minidriver/servo/xmlrpc"
)

// Servo holds the servod connection information.
type Servo struct {
	xmlrpc *xmlrpc.XMLRpc

	// Cache queried attributes that won't change.
	version   string
	servoType string
	hasCCD    bool

	removedWatchdogs []WatchdogValue
}

// New creates a new Servo object for communicating with a servod instance.
// connSpec holds servod's location, either as "host:port" or just "host"
// (to use the default port).
func New(ctx context.Context, host string, port int) (*Servo, error) {
	s := &Servo{xmlrpc: xmlrpc.New(host, port)}

	return s, nil
}

// Close performs Servo cleanup.
func (s *Servo) Close(ctx context.Context) error {
	var firstError error
	for _, v := range s.removedWatchdogs {
		logging.Infof(ctx, "Restoring servo watchdog %q", v)
		if err := s.WatchdogAdd(ctx, v); err != nil && firstError == nil {
			firstError = errors.Wrapf(err, "restoring watchdog %q", v)
		}
	}
	return firstError
}
