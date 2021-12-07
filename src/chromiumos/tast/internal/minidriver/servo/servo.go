// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

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

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/minidriver/servo/xmlrpc"
)

// Servo holds the servod connection information.
type Servo struct {
	xmlrpc *xmlrpc.XMLRpc

	// Cache queried attributes that won't change.
	version       string
	dutConnType   DUTConnTypeValue
	servoType     string
	hasCCD        bool
	hasServoMicro bool
	hasC2D2       bool
	isDualV4      bool

	// If initialPDRole is set, then upon Servo.Close(), the PDRole control will be set to initialPDRole.
	initialPDRole PDRoleValue

	removedWatchdogs []WatchdogValue
}

const (
	// servodDefaultHost is the default host for servod.
	servodDefaultHost = "localhost"
	// servodDefaultPort is the default port for servod.
	servodDefaultPort = 9999
)

// New creates a new Servo object for communicating with a servod instance.
// connSpec holds servod's location, either as "host:port" or just "host"
// (to use the default port).
func New(ctx context.Context, host string, port int) (*Servo, error) {
	s := &Servo{xmlrpc: xmlrpc.New(host, port)}

	return s, nil
	// Ensure Servo is set up properly before returning.
	// Not to verify connectivity for now.
	// Caller will verify later.
	// return s, s.verifyConnectivity(ctx)
}

// Default creates a Servo object for communicating with a local servod
// instance using the default port.
func Default(ctx context.Context) (*Servo, error) {
	return New(ctx, servodDefaultHost, servodDefaultPort)
}

// VerifyConnectivity sends and verifies an echo request to make sure
// everything is set up properly.
func (s *Servo) VerifyConnectivity(ctx context.Context) error {
	const msg = "hello from servo"
	actualMessage, err := s.Echo(ctx, "hello from servo")
	if err != nil {
		return err
	}

	const expectedMessage = "ECH0ING: " + msg
	if actualMessage != expectedMessage {
		return errors.Errorf("echo verification request returned %q; expected %q", actualMessage, expectedMessage)
	}

	return nil
}

// Close performs Servo cleanup.
func (s *Servo) Close(ctx context.Context) error {
	var firstError error
	if s.initialPDRole != "" && s.initialPDRole != PDRoleNA {
		logging.Infof(ctx, "Restoring %q to %q", PDRole, s.initialPDRole)
		if err := s.SetPDRole(ctx, s.initialPDRole); err != nil && firstError == nil {
			firstError = errors.Wrapf(err, "restoring servo control %q to %q", PDRole, s.initialPDRole)
		}
	}
	for _, v := range s.removedWatchdogs {
		logging.Infof(ctx, "Restoring servo watchdog %q", v)
		if err := s.WatchdogAdd(ctx, v); err != nil && firstError == nil {
			firstError = errors.Wrapf(err, "restoring watchdog %q", v)
		}
	}
	return firstError
}
