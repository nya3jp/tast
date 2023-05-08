// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// DO NOT USE THIS COPY OF SERVO IN TESTS, USE THE ONE IN platform/tast-tests/src/chromiumos/tast/common/servo

package servo

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/minidriver/servo/xmlrpc"
)

// A StringControl contains the name of a gettable/settable Control which takes a string value.
type StringControl string

// These are the Servo controls which can be get/set with a string value.
const (
	PowerState     StringControl = "power_state"
	Type           StringControl = "servo_type"
	WatchdogAdd    StringControl = "watchdog_add"
	WatchdogRemove StringControl = "watchdog_remove"
)

// A OnOffControl accepts either "on" or "off" as a value.
type OnOffControl string

// These controls accept only "on" and "off" as values.
const (
	CCDState OnOffControl = "ccd_state"
)

// An OnOffValue is a string value that would be accepted by an OnOffControl.
type OnOffValue string

// These are the values used by OnOff controls.
const (
	Off OnOffValue = "off"
	On  OnOffValue = "on"
)

// A PowerStateValue is a string accepted by the PowerState control.
type PowerStateValue string

// These are the string values that can be passed to the PowerState control.
const (
	PowerStateReset PowerStateValue = "reset"
)

// A WatchdogValue is a string that would be accepted by WatchdogAdd & WatchdogRemove control.
type WatchdogValue string

// These are the string watchdog type values that can be passed to WatchdogAdd & WatchdogRemove.
const (
	WatchdogCCD  WatchdogValue = "ccd"
	WatchdogMain WatchdogValue = "main"
)

// DUTController is the active controller on a dual mode servo.
type DUTController string

// Parameters that can be passed to SetActiveDUTController().
const (
	DUTControllerC2D2       DUTController = "c2d2"
	DUTControllerCCD        DUTController = "ccd_cr50"
	DUTControllerServoMicro DUTController = "servo_micro"
)

// HasControl determines whether the Servo being used supports the given control.
func (s *Servo) HasControl(ctx context.Context, ctrl string) (bool, error) {
	err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("doc", ctrl))
	// If the control exists, doc() should return with no issue.
	if err == nil {
		return true, nil
	}
	// If the control doesn't exist, then doc() should return a fault.
	if _, isFault := err.(xmlrpc.FaultError); isFault {
		return false, nil
	}
	// A non-fault error indicates that something went wrong.
	return false, err
}

// GetServoVersion gets the version of Servo being used.
func (s *Servo) GetServoVersion(ctx context.Context) (string, error) {
	if s.version != "" {
		return s.version, nil
	}
	err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("get_version"), &s.version)
	return s.version, err
}

// IsServoV4 determines whether the Servo being used is v4.
func (s *Servo) IsServoV4(ctx context.Context) (bool, error) {
	version, err := s.GetServoVersion(ctx)
	if err != nil {
		return false, errors.Wrap(err, "determining servo version")
	}
	return strings.HasPrefix(version, "servo_v4"), nil
}

// GetString returns the value of a specified control.
func (s *Servo) GetString(ctx context.Context, control StringControl) (string, error) {
	var value string
	if err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("get", string(control)), &value); err != nil {
		return "", errors.Wrapf(err, "getting value for servo control %q", control)
	}
	return value, nil
}

// SetString sets a Servo control to a string value.
func (s *Servo) SetString(ctx context.Context, control StringControl, value string) error {
	// Servo's Set method returns a bool stating whether the call succeeded or not.
	// This is redundant, because a failed call will return an error anyway.
	// So, we can skip unpacking the output.
	if err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("set", string(control), value)); err != nil {
		return errors.Wrapf(err, "setting servo control %q to %q", control, value)
	}
	return nil
}

// SetStringTimeout sets a Servo control to a string value.
func (s *Servo) SetStringTimeout(ctx context.Context, control StringControl, value string, timeout time.Duration) error {
	// Servo's Set method returns a bool stating whether the call succeeded or not.
	// This is redundant, because a failed call will return an error anyway.
	// So, we can skip unpacking the output.
	if err := s.xmlrpc.Run(ctx, xmlrpc.NewCallTimeout("set", timeout, string(control), value)); err != nil {
		return errors.Wrapf(err, "setting servo control %q to %q", control, value)
	}
	return nil
}

// SetPowerState sets the PowerState control.
// Because this is particularly disruptive, it is always logged.
// It can be slow, because some boards are configured to hold down the power button for 12 seconds.
func (s *Servo) SetPowerState(ctx context.Context, value PowerStateValue) error {
	logging.Infof(ctx, "Setting %q to %q", PowerState, value)
	// Power states that reboot the EC can make servod exit or fail if the CCD watchdog is enabled.
	switch value {
	case PowerStateReset:
		if err := s.WatchdogRemove(ctx, WatchdogCCD); err != nil {
			return errors.Wrap(err, "remove ccd watchdog")
		}
	default:
		// Do nothing
	}
	return s.SetStringTimeout(ctx, PowerState, string(value), 30*time.Second)
}

// GetOnOff gets an OnOffControl as a bool.
func (s *Servo) GetOnOff(ctx context.Context, ctrl OnOffControl) (bool, error) {
	str, err := s.GetString(ctx, StringControl(ctrl))
	if err != nil {
		return false, err
	}
	switch str {
	case string(On):
		return true, nil
	case string(Off):
		return false, nil
	}
	return false, errors.Errorf("cannot convert %q to boolean", str)
}

// WatchdogAdd adds the specified watchdog to the servod instance.
func (s *Servo) WatchdogAdd(ctx context.Context, val WatchdogValue) error {
	return s.SetString(ctx, WatchdogAdd, string(val))
}

// WatchdogRemove removes the specified watchdog from the servod instance.
// Servo.Close() will restore the watchdog.
func (s *Servo) WatchdogRemove(ctx context.Context, val WatchdogValue) error {
	if val == WatchdogCCD {
		// SuzyQ reports as ccd_cr50, and doesn't have a watchdog named CCD.
		servoType, err := s.GetServoType(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get servo type")
		}
		// No need to remove CCD watchdog if there is no CCD.
		if !s.hasCCD {
			logging.Info(ctx, "Skipping watchdog remove CCD, because there is no CCD")
			return nil
		}
		if servoType == "ccd_cr50" {
			val = WatchdogMain
		}
	}
	if err := s.SetString(ctx, WatchdogRemove, string(val)); err != nil {
		return err
	}
	s.removedWatchdogs = append(s.removedWatchdogs, val)
	return nil
}

// GetServoType gets the type of the servo.
func (s *Servo) GetServoType(ctx context.Context) (string, error) {
	if s.servoType != "" {
		return s.servoType, nil
	}
	servoType, err := s.GetString(ctx, Type)
	if err != nil {
		return "", err
	}
	hasCCD := strings.Contains(servoType, string(DUTControllerCCD))
	if !hasCCD {
		if hasCCDState, err := s.HasControl(ctx, string(CCDState)); err != nil {
			return "", errors.Wrap(err, "failed to check ccd_state control")
		} else if hasCCDState {
			ccdState, err := s.GetOnOff(ctx, CCDState)
			if err != nil {
				return "", errors.Wrap(err, "failed to get ccd_state")
			}
			hasCCD = ccdState
		}
	}

	s.servoType = servoType
	s.hasCCD = hasCCD
	return s.servoType, nil
}
