// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver

import (
	"context"

	"chromiumos/tast/cmd/tast/internal/run/config"
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
func (a *DUTActionServo) HardReboot(ctx context.Context) error {
	// Don't do anything for now. We will use servo in the next CL.
	return nil
}
