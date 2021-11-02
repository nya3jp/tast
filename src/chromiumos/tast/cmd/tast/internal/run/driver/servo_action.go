// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver

import (
	"context"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/driver/internal/servo"
	"chromiumos/tast/errors"
)

// DUTActionServo stores information for connection to servo host.
type DUTActionServo struct {
	servoSpec string
	cfg       *config.Config
}

// NewDUTActionServo return a handler that allow users to manipulate DUT
// through servo.
func NewDUTActionServo(cfg *config.Config, servoHostPort string) (action *DUTActionServo, err error) {
	return &DUTActionServo{servoSpec: servoHostPort, cfg: cfg}, nil
}

// HardReboot will reboot DUT by using servo.
// Disclaimer: It is simply a best-effort attempt to reboot a DUT in tast session.
// During a repair task, we have a full set of verify/repair actions against servo
// itself to ensure it in the best state it can. However, the recover logic is
// complex and consume more time so we only do a simple reset here.
func (a *DUTActionServo) HardReboot(ctx context.Context) error {
	// Connect to servo.
	servoSpec := a.servoSpec
	pxy, err := servo.NewProxy(ctx, servoSpec, a.cfg.KeyFile(), a.cfg.KeyDir())
	if err != nil {
		return errors.Wrapf(err, "Failed to connect to servo host %s", servoSpec)
	}
	defer pxy.Close(ctx)
	svo := pxy.Servo()
	if err := svo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		return errors.Wrapf(err, "Failed to reboot DUT through servo host %s", servoSpec)
	}
	return nil
}
