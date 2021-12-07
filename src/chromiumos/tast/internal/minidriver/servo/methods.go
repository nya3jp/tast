// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package servo

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/minidriver/servo/xmlrpc"
)

// A StringControl contains the name of a gettable/settable Control which takes a string value.
type StringControl string

// These are the Servo controls which can be get/set with a string value.
const (
	ActiveChgPort         StringControl = "active_chg_port"
	ActiveDUTController   StringControl = "active_dut_controller"
	DownloadImageToUSBDev StringControl = "download_image_to_usb_dev"
	DUTVoltageMV          StringControl = "dut_voltage_mv"
	FWWPState             StringControl = "fw_wp_state"
	ImageUSBKeyDirection  StringControl = "image_usbkey_direction"
	ImageUSBKeyPwr        StringControl = "image_usbkey_pwr"
	ImageUSBKeyDev        StringControl = "image_usbkey_dev"
	LidOpen               StringControl = "lid_open"
	PowerState            StringControl = "power_state"
	Type                  StringControl = "servo_type"
	UARTCmd               StringControl = "servo_v4_uart_cmd"
	UARTCmdV4p1           StringControl = "servo_v4p1_uart_cmd"
	Watchdog              StringControl = "watchdog"
	WatchdogAdd           StringControl = "watchdog_add"
	WatchdogRemove        StringControl = "watchdog_remove"

	// DUTConnectionType was previously known as V4Type ("servo_v4_type")
	DUTConnectionType StringControl = "root.dut_connection_type"

	// PDRole was previously known as V4Role ("servo_v4_role")
	PDRole StringControl = "servo_pd_role"
)

// A BoolControl contains the name of a gettable/settable Control which takes a boolean value.
type BoolControl string

// These are the Servo controls which can be get/set with a boolean value.
const (
	ChargerAttached BoolControl = "charger_attached"
)

// An IntControl contains the name of a gettable/settable Control which takes an integer value.
type IntControl string

// These are the Servo controls which can be get/set with an integer value.
const (
	BatteryChargeMAH     IntControl = "battery_charge_mah"
	BatteryCurrentMA     IntControl = "ppvar_vbat_ma"
	BatteryFullChargeMAH IntControl = "battery_full_charge_mah"
	BatteryVoltageMV     IntControl = "ppvar_vbat_mv"
	VolumeDownHold       IntControl = "volume_down_hold"    // Integer represents a number of milliseconds.
	VolumeUpHold         IntControl = "volume_up_hold"      // Integer represents a number of milliseconds.
	VolumeUpDownHold     IntControl = "volume_up_down_hold" // Integer represents a number of milliseconds.
)

// A FloatControl contains the name of a gettable/settable Control which takes a floating-point value.
type FloatControl string

// These are the Servo controls with floating-point values.
const (
	BatteryTemperatureCelsius FloatControl = "battery_tempc"
	VBusVoltage               FloatControl = "vbus_voltage"
)

// A OnOffControl accepts either "on" or "off" as a value.
type OnOffControl string

// These controls accept only "on" and "off" as values.
const (
	CCDKeepaliveEn OnOffControl = "ccd_keepalive_en"
	CCDState       OnOffControl = "ccd_state"
	DTSMode        OnOffControl = "servo_dts_mode"
	RecMode        OnOffControl = "rec_mode"
)

// An OnOffValue is a string value that would be accepted by an OnOffControl.
type OnOffValue string

// These are the values used by OnOff controls.
const (
	Off OnOffValue = "off"
	On  OnOffValue = "on"
)

// A KeypressControl is a special type of Control which can take either a numerical value or a KeypressDuration.
type KeypressControl StringControl

// These are the Servo controls which can be set with either a numerical value or a KeypressDuration.
const (
	CtrlD        KeypressControl = "ctrl_d"
	CtrlS        KeypressControl = "ctrl_s"
	CtrlU        KeypressControl = "ctrl_u"
	CtrlEnter    KeypressControl = "ctrl_enter"
	Ctrl         KeypressControl = "ctrl_key"
	Enter        KeypressControl = "enter_key"
	Refresh      KeypressControl = "refresh_key"
	CtrlRefresh  KeypressControl = "ctrl_refresh_key"
	ImaginaryKey KeypressControl = "imaginary_key"
	SysRQX       KeypressControl = "sysrq_x"
	PowerKey     KeypressControl = "power_key"
	Pwrbutton    KeypressControl = "pwr_button"
	USBEnter     KeypressControl = "usb_keyboard_enter_key"
)

// A KeypressDuration is a string accepted by a KeypressControl.
type KeypressDuration string

// These are string values that can be passed to a KeypressControl.
const (
	DurTab       KeypressDuration = "tab"
	DurPress     KeypressDuration = "press"
	DurLongPress KeypressDuration = "long_press"
)

// Dur returns a custom duration that can be passed to KeypressWithDuration
func Dur(dur time.Duration) KeypressDuration {
	return KeypressDuration(fmt.Sprintf("%f", dur.Seconds()))
}

// A FWWPStateValue is a string accepted by the FWWPState control.
type FWWPStateValue string

// These are the string values that can be passed to the FWWPState control.
const (
	FWWPStateOff FWWPStateValue = "force_off"
	FWWPStateOn  FWWPStateValue = "force_on"
)

// A LidOpenValue is a string accepted by the LidOpen control.
type LidOpenValue string

// These are the string values that can be passed to the LidOpen control.
const (
	LidOpenYes LidOpenValue = "yes"
	LidOpenNo  LidOpenValue = "no"
)

// A PowerStateValue is a string accepted by the PowerState control.
type PowerStateValue string

// These are the string values that can be passed to the PowerState control.
const (
	PowerStateCR50Reset   PowerStateValue = "cr50_reset"
	PowerStateOff         PowerStateValue = "off"
	PowerStateOn          PowerStateValue = "on"
	PowerStateRec         PowerStateValue = "rec"
	PowerStateRecForceMRC PowerStateValue = "rec_force_mrc"
	PowerStateReset       PowerStateValue = "reset"
	PowerStateWarmReset   PowerStateValue = "warm_reset"
)

// A USBMuxState indicates whether the servo's USB mux is on, and if so, which direction it is powering.
type USBMuxState string

// These are the possible states of the USB mux.
const (
	USBMuxOff  USBMuxState = "off"
	USBMuxDUT  USBMuxState = "dut_sees_usbkey"
	USBMuxHost USBMuxState = "servo_sees_usbkey"
)

// A PDRoleValue is a string that would be accepted by the PDRole control.
type PDRoleValue string

// These are the string values that can be passed to PDRole.
const (
	PDRoleSnk PDRoleValue = "snk"
	PDRoleSrc PDRoleValue = "src"

	// PDRoleNA indicates a non-v4 servo.
	PDRoleNA PDRoleValue = "n/a"
)

// A DUTConnTypeValue is a string that would be returned by the DUTConnectionType control.
type DUTConnTypeValue string

// These are the string values that can be returned by DUTConnectionType
const (
	DUTConnTypeA DUTConnTypeValue = "type-a"
	DUTConnTypeC DUTConnTypeValue = "type-c"

	// DUTConnTypeNA indicates a non-v4 servo.
	DUTConnTypeNA DUTConnTypeValue = "n/a"
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

// ServoKeypressDelay comes from hdctools/servo/drv/keyboard_handlers.py.
// It is the minimum time interval between 'press' and 'release' keyboard events.
const ServoKeypressDelay = 100 * time.Millisecond

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

// Echo calls the Servo echo method.
func (s *Servo) Echo(ctx context.Context, message string) (string, error) {
	var val string
	err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("echo", message), &val)
	return val, err
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

// GetBool returns the boolean value of a specified control.
func (s *Servo) GetBool(ctx context.Context, control BoolControl) (bool, error) {
	var value bool
	if err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("get", string(control)), &value); err != nil {
		return false, errors.Wrapf(err, "getting value for servo control %q", control)
	}
	return value, nil
}

// parseUint extracts a hex number from `value` at `*index+1` that is exactly `bits` in length.
// `bits` must be power of 2.
// `*index` will be moved to the end of the extracted runes.
func parseUint(value []rune, index *int, bits int) (rune, error) {
	chars := bits / 4
	endIndex := *index + chars
	if endIndex >= len(value) {
		return 0, errors.Errorf("unparsable escape sequence `\\%s`", string(value[*index:]))
	}
	char, err := strconv.ParseUint(string(value[*index+1:endIndex+1]), 16, bits)
	if err != nil {
		return 0, errors.Wrapf(err, "unparsable escape sequence `\\%s`", string(value[*index:endIndex+1]))
	}
	*index += chars
	return rune(char), nil
}

// parseQuotedStringInternal returns a new string with the quotes and escaped chars from `value` removed, moves `*index` to the index of the closing quote rune.
func parseQuotedStringInternal(value []rune, index *int) (string, error) {
	if *index >= len(value) {
		return "", errors.Errorf("unexpected end of string at %d in %s", *index, string(value))
	}
	// The first char should always be a ' or "
	quoteChar := value[*index]
	if quoteChar != '\'' && quoteChar != '"' {
		return "", errors.Errorf("unexpected string char %c at index %d in %s", quoteChar, *index, string(value))
	}
	(*index)++
	var current strings.Builder
	for ; *index < len(value); (*index)++ {
		c := value[*index]
		if c == quoteChar {
			break
		} else if c == '\\' {
			(*index)++
			if *index >= len(value) {
				return "", errors.New("unparsable escape sequence \\")
			}
			switch value[*index] {
			case '"', '\'', '\\':
				current.WriteRune(value[*index])
			case 'r':
				current.WriteRune('\r')
			case 'n':
				current.WriteRune('\n')
			case 't':
				current.WriteRune('\t')
			case 'x':
				r, err := parseUint(value, index, 8)
				if err != nil {
					return "", err
				}
				current.WriteRune(r)
			case 'u':
				r, err := parseUint(value, index, 16)
				if err != nil {
					return "", err
				}
				current.WriteRune(r)
			case 'U':
				r, err := parseUint(value, index, 32)
				if err != nil {
					return "", err
				}
				current.WriteRune(r)
			default:
				return "", errors.Errorf("unexpected escape sequence \\%c at index %d in %s", value[*index], *index, string(value))
			}
		} else {
			current.WriteRune(c)
		}
	}
	return current.String(), nil
}

// parseStringListInternal parses `value` as a possibly nested list of strings, each quoted and separated by commas. Moves `*index` to the index of the closing ] rune.
func parseStringListInternal(value []rune, index *int) ([]interface{}, error) {
	var result []interface{}
	if *index >= len(value) {
		return nil, errors.Errorf("unexpected end of string at %d in %s", *index, string(value))
	}
	// The first char should always be a [ or (, as it might be a list or a tuple.
	if value[*index] != '[' && value[*index] != '(' {
		return nil, errors.Errorf("unexpected list char %c at index %d in %s", value[*index], *index, string(value))
	}
	(*index)++
	for ; *index < len(value); (*index)++ {
		c := value[*index]
		switch c {
		case '[', '(':
			sublist, err := parseStringListInternal(value, index)
			if err != nil {
				return nil, err
			}
			result = append(result, sublist)
		case '\'', '"':
			substr, err := parseQuotedStringInternal(value, index)
			if err != nil {
				return nil, err
			}
			result = append(result, substr)
		case ',', ' ':
			// Ignore this char
		case ']', ')':
			return result, nil
		default:
			return nil, errors.Errorf("unexpected list char %c at index %d in %s", c, *index, string(value))
		}
	}
	return nil, errors.Errorf("unexpected end of string at %d in %s", *index, string(value))
}

// ParseStringList parses `value` as a possibly nested list of strings, each quoted and separated by commas.
func ParseStringList(value string) ([]interface{}, error) {
	index := 0
	return parseStringListInternal([]rune(value), &index)
}

// ParseQuotedString returns a new string with the quotes and escaped chars from `value` removed.
func ParseQuotedString(value string) (string, error) {
	index := 0
	return parseQuotedStringInternal([]rune(value), &index)
}

// GetStringList parses the value of a control as an encoded list
func (s *Servo) GetStringList(ctx context.Context, control StringControl) ([]interface{}, error) {
	v, err := s.GetString(ctx, control)
	if err != nil {
		return nil, err
	}
	return ParseStringList(v)
}

// GetQuotedString parses the value of a control as a quoted string
func (s *Servo) GetQuotedString(ctx context.Context, control StringControl) (string, error) {
	v, err := s.GetString(ctx, control)
	if err != nil {
		return "", err
	}
	return ParseQuotedString(v)
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

// SetInt sets a Servo control to an integer value.
func (s *Servo) SetInt(ctx context.Context, control IntControl, value int) error {
	if err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("set", string(control), value)); err != nil {
		return errors.Wrapf(err, "setting servo control %q to %d", control, value)
	}
	return nil
}

// GetInt returns the integer value of a specified control.
func (s *Servo) GetInt(ctx context.Context, control IntControl) (int, error) {
	var value int
	if err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("get", string(control)), &value); err != nil {
		return 0, errors.Wrapf(err, "getting value for servo control %q", control)
	}
	return value, nil
}

// GetFloat returns the floating-point value of a specified control.
func (s *Servo) GetFloat(ctx context.Context, control FloatControl) (float64, error) {
	var value float64
	if err := s.xmlrpc.Run(ctx, xmlrpc.NewCall("get", string(control)), &value); err != nil {
		return 0, errors.Wrapf(err, "getting value for servo control %q", control)
	}
	return value, nil
}

// SetPowerState sets the PowerState control.
// Because this is particularly disruptive, it is always logged.
// It can be slow, because some boards are configured to hold down the power button for 12 seconds.
func (s *Servo) SetPowerState(ctx context.Context, value PowerStateValue) error {
	logging.Infof(ctx, "Setting %q to %q", PowerState, value)
	// Power states that reboot the EC can make servod exit or fail if the CCD watchdog is enabled.
	switch value {
	case PowerStateReset, PowerStateRec, PowerStateRecForceMRC, PowerStateCR50Reset:
		if err := s.WatchdogRemove(ctx, WatchdogCCD); err != nil {
			return errors.Wrap(err, "remove ccd watchdog")
		}
	default:
		// Do nothing
	}
	return s.SetStringTimeout(ctx, PowerState, string(value), 30*time.Second)
}

// GetPDRole returns the servo's current PDRole (SNK or SRC), or PDRoleNA if Servo is not V4.
func (s *Servo) GetPDRole(ctx context.Context) (PDRoleValue, error) {
	isV4, err := s.IsServoV4(ctx)
	if err != nil {
		return "", errors.Wrap(err, "determining whether servo is v4")
	}
	if !isV4 {
		return PDRoleNA, nil
	}
	role, err := s.GetString(ctx, PDRole)
	if err != nil {
		return "", err
	}
	return PDRoleValue(role), nil
}

// SetPDRole sets the PDRole control for a servo v4.
// On a Servo version other than v4, this does nothing.
func (s *Servo) SetPDRole(ctx context.Context, newRole PDRoleValue) error {
	// Determine the current PD role
	currentRole, err := s.GetPDRole(ctx)
	if err != nil {
		return errors.Wrap(err, "getting current PD role")
	}

	// Save the initial PD role so we can restore it during servo.Close()
	if s.initialPDRole == "" {
		logging.Infof(ctx, "Saving initial PDRole %q for later", currentRole)
		s.initialPDRole = currentRole
	}

	// If not using a servo V4, then we can't set the PD Role
	if currentRole == PDRoleNA {
		logging.Infof(ctx, "Skipping setting %q to %q on non-v4 servo", PDRole, newRole)
		return nil
	}

	// If the current value is already the intended value,
	// then don't bother resetting.
	if currentRole == newRole {
		logging.Infof(ctx, "Skipping setting %q to %q, because that is the current value", PDRole, newRole)
		return nil
	}

	return s.SetString(ctx, PDRole, string(newRole))
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
	hasServoMicro := strings.Contains(servoType, string(DUTControllerServoMicro))
	hasC2D2 := strings.Contains(servoType, string(DUTControllerC2D2))
	isDualV4 := strings.Contains(servoType, "_and_")

	if !hasCCD && !hasServoMicro && !hasC2D2 {
		logging.Infof(ctx, "Assuming %s is equivalent to servo_micro", servoType)
		hasServoMicro = true
	}
	s.servoType = servoType
	s.hasCCD = hasCCD
	s.hasServoMicro = hasServoMicro
	s.hasC2D2 = hasC2D2
	s.isDualV4 = isDualV4
	return s.servoType, nil
}

// HasCCD checks if the servo has a CCD connection.
func (s *Servo) HasCCD(ctx context.Context) (bool, error) {
	_, err := s.GetServoType(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get servo type")
	}

	return s.hasCCD, nil
}
