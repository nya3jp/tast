// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package target

import (
	"context"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/minidriver/externalservers"
	"chromiumos/tast/internal/minidriver/servo"
	"chromiumos/tast/internal/protocol"
)

// dutHelper provide utilities to perform operation on DUt.
type dutHelper struct {
	servoSpec string // servoSpec stores servo spec
	dutServer string // dutServer stores DUT server information.
	cfg       *protocol.SSHConfig
	testVars  map[string]string
}

// newDUTHelper return a helper that allow users to manipulate DUT.
func newDUTHelper(ctx context.Context, cfg *protocol.SSHConfig, testVars map[string]string, role string) *dutHelper {
	return &dutHelper{
		servoSpec: servoHost(ctx, role, testVars),
		dutServer: dutHost(ctx, role, testVars),
		cfg:       cfg,
	}
}

// servoHost finds servo related information for a particular role from a variable to value map.
func servoHost(ctx context.Context, role string, testVars map[string]string) string {
	if servoVarVal, ok := testVars["servers.servo"]; ok {
		roleToServer, err := externalservers.ParseServerVarValues(servoVarVal)
		if err == nil {
			server, ok := roleToServer[role]
			if !ok {
				logging.Infof(ctx, "No servo server information for role: %s", role)
			}
			return server
		}
		logging.Infof(ctx, "Failed to parse servo server information: %v", err)
	}
	if role != "" {
		return ""
	}
	// Fallback to the old way for primary dut.
	if servoVarVal, ok := testVars["servo"]; ok {
		return servoVarVal
	}
	return ""
}

// dutHost finds DUT server related information for a particular role from a variable to value map.
func dutHost(ctx context.Context, role string, testVars map[string]string) string {
	if dutVarVal, ok := testVars["servers.dut"]; ok {
		roleToServer, err := externalservers.ParseServerVarValues(dutVarVal)
		if err == nil {
			server, ok := roleToServer[role]
			if !ok {
				logging.Infof(ctx, "No DUT server information for role: %s", role)
			}
			return server
		}
		logging.Infof(ctx, "Failed to parse dut server information: %v", err)
	}
	return ""
}

// HardReboot will reboot DUT by using servo.
// Disclaimer: It is simply a best-effort attempt to reboot a DUT in tast session.
// During a repair task, we have a full set of verify/repair actions against servo
// itself to ensure it in the best state it can. However, the recover logic is
// complex and consume more time so we only do a simple reset here.
// TODO: b/207587837 create a unit test forthis HardReboot function.
func (a *dutHelper) HardReboot(ctx context.Context) error {
	if a.servoSpec == "" {
		return errors.New("reboot service is not available")
	}
	// Connect to servo.
	servoSpec := a.servoSpec
	pxy, err := servo.NewProxy(ctx, servoSpec, a.cfg.GetKeyFile(), a.cfg.GetKeyDir())
	if err != nil {
		return errors.Wrapf(err, "failed to connect to servo host %s", servoSpec)
	}
	defer pxy.Close(ctx)
	svo := pxy.Servo()
	if err := svo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		return errors.Wrapf(err, "failed to reboot DUT through servo host %s", servoSpec)
	}
	return nil
}
