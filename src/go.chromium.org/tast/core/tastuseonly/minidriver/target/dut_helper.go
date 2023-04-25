// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package target

import (
	"context"
	"net"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast/core/tastuseonly/logging"
	"go.chromium.org/tast/core/tastuseonly/minidriver/externalservers"
	"go.chromium.org/tast/core/tastuseonly/minidriver/servo"
	"go.chromium.org/tast/core/tastuseonly/protocol"
)

// dutHelper provide utilities to perform operation on DUt.
type dutHelper struct {
	servoSpec string // servoSpec stores servo spec
	dutServer string // dutServer stores DUT server information.
	cfg       *protocol.SSHConfig
	testVars  map[string]string
}

// newDUTHelper return a helper that allow users to manipulate DUT.
// using NewProxy to establish servo communication
func newDUTHelper(ctx context.Context, cfg *protocol.SSHConfig, testVars map[string]string, role string) *dutHelper {
	a := dutHelper{
		servoSpec: ServoHost(ctx, role, testVars),
		dutServer: dutHost(ctx, role, testVars),
		cfg:       cfg,
	}
	if a.servoSpec == "" {
		return &a
	}
	if err := ensureDUTConnection(ctx, cfg.ConnectionSpec); err != nil {
		logging.Infof(ctx, "Failed to connect to DUT %s", err)
	}
	return &a
}

// ensureDUTConnection poll for a minute to make sure DUT connection is ready
// to prevent the accidental disconnection of the DUT after the servod is established.
func ensureDUTConnection(ctx context.Context, target string) error {
	logging.Infof(ctx, "Wait for DUT connection to be ready")
	opts := &ssh.Options{}
	if err := ssh.ParseTarget(target, opts); err != nil {
		return err
	}
	host, port, err := net.SplitHostPort(opts.Hostname)
	if err != nil {
		return err
	}
	// Poll for a minute to make sure DUT connection is ready.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := tryResolveDNS(ctx, host); err != nil {
			return errors.Wrap(err, "* DNS resolution: FAIL: ")
		}
		if err := tryPing(ctx, host); err != nil {
			return errors.Wrap(err, "* Ping: FAIL: ")
		}
		if err := tryRawConnect(ctx, host, port); err != nil {
			return errors.Wrap(err, "* Connect: FAIL: ")
		}
		return nil
	}, &testing.PollOptions{Interval: 10 * time.Second,
		Timeout: time.Minute}); err != nil {
		logging.Infof(ctx, "DUT connection testing poll fail: %v", err)
	}
	return nil
}

// ServoHost finds servo related information for a particular role from a variable to value map.
func ServoHost(ctx context.Context, role string, testVars map[string]string) string {
	if servoVarVal, ok := testVars["servers.servo"]; ok {
		roleToServer, err := externalservers.ParseServerVarValues(servoVarVal)
		if err == nil {
			server, ok := roleToServer[role]
			if ok {
				return server
			}
			logging.Infof(ctx, "No servo server information for role: %s", role)
		} else {
			logging.Infof(ctx, "Failed to parse servo server information: %v", err)
		}
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
			if ok {
				return server
			}
			logging.Infof(ctx, "No DUT server information for role: %s", role)
		} else {
			logging.Infof(ctx, "Failed to parse dut server information: %v", err)
		}
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
