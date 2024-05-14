// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crosbundle

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	configpb "go.chromium.org/chromiumos/config/go/api"
	softwarepb "go.chromium.org/chromiumos/config/go/api/software"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/testing/wlan"

	"go.chromium.org/tast/core/framework/protocol"
)

// GSCKeyID is a hex value that represents a key used to sign a GSC image.
type GSCKeyID string

// prodRWGSCKeyIDs is a slice with production keyIDs used to sign the RW GSC image.
var prodRWGSCKeyIDs = []GSCKeyID{"0x87b73b67", "0xde88588d"}

func crosConfig(path, prop string) (string, error) {
	cmd := exec.Command("cros_config", path, prop)
	var buf bytes.Buffer
	cmd.Stderr = &buf
	b, err := cmd.Output()
	if err != nil {
		if xerr, ok := err.(*exec.ExitError); !ok || xerr.ExitCode() != 1 {
			return "", errors.Errorf("cros_config failed (stderr: %q): %v", buf.Bytes(), err)
		}
	}
	return string(b), nil
}

// detectHardwareFeatures returns a device.Config and api.HardwareFeatures instances
// some of whose members are filled based on runtime information.
func detectHardwareFeatures(ctx context.Context) (*protocol.HardwareFeatures, error) {
	platform, err := func() (string, error) {
		out, err := crosConfig("/identity", "platform-name")
		if err != nil {
			return "", err
		}
		return out, nil
	}()
	if err != nil {
		logging.Infof(ctx, "Unknown platform-id: %v", err)
	}
	model, err := func() (string, error) {
		out, err := crosConfig("/", "name")
		if err != nil {
			return "", err
		}
		return out, nil
	}()
	if err != nil {
		logging.Infof(ctx, "Unknown model-id: %v", err)
	}
	brand, err := func() (string, error) {
		out, err := crosConfig("/", "brand-code")
		if err != nil {
			return "", err
		}
		return out, nil
	}()
	if err != nil {
		logging.Infof(ctx, "Unknown brand-id: %v", err)
	}

	info, err := cpuInfo()
	if err != nil {
		logging.Infof(ctx, "Unknown CPU information: %v", err)
	}

	vboot2, err := func() (bool, error) {
		out, err := exec.Command("crossystem", "fw_vboot2").Output()
		if err != nil {
			return false, err
		}
		return strings.TrimSpace(string(out)) == "1", nil
	}()
	if err != nil {
		logging.Infof(ctx, "Unknown vboot2 info: %v", err)
	}

	hasSideVolumeButton, err := func() (bool, error) {
		out, err := crosConfig("/hardware-properties", "has-side-volume-button")
		if err != nil {
			return false, err
		}
		return strings.TrimSpace(string(out)) == "true", nil
	}()
	if err != nil {
		logging.Infof(ctx, "Unknown has-side-volume-button: %v", err)
	}

	config := &protocol.DeprecatedDeviceConfig{
		Id: &protocol.DeprecatedConfigId{
			Platform: platform,
			Model:    model,
			Brand:    brand,
		},
		Soc:                 info.soc,
		Cpu:                 info.cpuArch,
		HasNvmeSelfTest:     false,
		HasVboot2:           vboot2,
		HasSideVolumeButton: hasSideVolumeButton,
	}
	features := &configpb.HardwareFeatures{
		Screen:                  &configpb.HardwareFeatures_Screen{},
		Fingerprint:             &configpb.HardwareFeatures_Fingerprint{},
		EmbeddedController:      &configpb.HardwareFeatures_EmbeddedController{},
		Storage:                 &configpb.HardwareFeatures_Storage{},
		Memory:                  &configpb.HardwareFeatures_Memory{},
		Audio:                   &configpb.HardwareFeatures_Audio{},
		PrivacyScreen:           &configpb.HardwareFeatures_PrivacyScreen{},
		Soc:                     &configpb.HardwareFeatures_Soc{},
		Touchpad:                &configpb.HardwareFeatures_Touchpad{},
		Keyboard:                &configpb.HardwareFeatures_Keyboard{},
		FormFactor:              &configpb.HardwareFeatures_FormFactor{},
		DpConverter:             &configpb.HardwareFeatures_DisplayPortConverter{},
		Wifi:                    &configpb.HardwareFeatures_Wifi{},
		Cellular:                &configpb.HardwareFeatures_Cellular{},
		Bluetooth:               &configpb.HardwareFeatures_Bluetooth{},
		Hps:                     &configpb.HardwareFeatures_Hps{},
		Battery:                 &configpb.HardwareFeatures_Battery{},
		Camera:                  &configpb.HardwareFeatures_Camera{},
		TrustedPlatformModule:   &configpb.HardwareFeatures_TrustedPlatformModule{},
		FwConfig:                &configpb.HardwareFeatures_FirmwareConfiguration{},
		RuntimeProbeConfig:      &configpb.HardwareFeatures_RuntimeProbeConfig{},
		HardwareProbeConfig:     &configpb.HardwareFeatures_HardwareProbe{},
		Suspend:                 &configpb.HardwareFeatures_Suspend{},
		Display:                 &configpb.HardwareFeatures_Display{},
		Vrr:                     &configpb.HardwareFeatures_Vrr{},
		Hdmi:                    &configpb.HardwareFeatures_Hdmi{},
		InterruptControllerInfo: &configpb.HardwareFeatures_InterruptControllerInfo{},
		TiledDisplay:            &configpb.HardwareFeatures_TiledDisplay{},
	}

	swConfig := &softwarepb.SoftwareConfig{
		FirmwareInfo: &softwarepb.FirmwareInfo{},
	}

	formFactor, err := func() (string, error) {
		out, err := crosConfig("/hardware-properties", "form-factor")
		if err != nil {
			return "", err
		}
		return out, nil
	}()
	if err != nil {
		logging.Infof(ctx, "Unknown /hardware-properties/form-factor: %v", err)
	}
	lidConvertible, err := func() (string, error) {
		out, err := crosConfig("/hardware-properties", "is-lid-convertible")
		if err != nil {
			return "", err
		}
		return out, nil
	}()
	if err != nil {
		logging.Infof(ctx, "Unknown /hardware-properties/is-lid-convertible: %v", err)
	}
	features.Wifi, err = wifiFeatures()
	if err != nil {
		logging.Infof(ctx, "Error getting Wifi: %v", err)
	}

	// Battery
	noBatteryBootSupported, err := func() (bool, error) {
		out, err := crosConfig("/battery", "no-battery-boot-supported")
		if err != nil {
			return false, err
		}
		return out == "true", nil
	}()
	features.Battery.NoBatteryBootSupported = noBatteryBootSupported

	detachableBasePath, err := func() (string, error) {
		out, err := crosConfig("/detachable-base", "usb-path")
		if err != nil {
			return "", err
		}
		return out, nil
	}()
	if err != nil {
		logging.Infof(ctx, "Unknown /detachable-base/usbpath: %v", err)
	}

	checkHammer := func() bool {
		hammer, err := exec.Command("udevadm", "info", "--export-db").Output()
		if err != nil {
			return false
		}
		return regexp.MustCompile(`(?m)^E: ID_MODEL=Hammer$`).Match(hammer)
	}

	if formFactorEnum, ok := configpb.HardwareFeatures_FormFactor_FormFactorType_value[formFactor]; ok {
		features.FormFactor.FormFactor = configpb.HardwareFeatures_FormFactor_FormFactorType(formFactorEnum)
	} else if formFactor == "CHROMEBOOK" {
		// Gru devices have formFactor=="CHROMEBOOK", detachableBasePath=="", lidConvertible="", but are really CHROMESLATE
		if platform == "Gru" {
			features.FormFactor.FormFactor = configpb.HardwareFeatures_FormFactor_CHROMESLATE
		} else if detachableBasePath != "" {
			features.FormFactor.FormFactor = configpb.HardwareFeatures_FormFactor_DETACHABLE
		} else if lidConvertible == "true" {
			features.FormFactor.FormFactor = configpb.HardwareFeatures_FormFactor_CONVERTIBLE
		} else {
			features.FormFactor.FormFactor = configpb.HardwareFeatures_FormFactor_CLAMSHELL
		}
	} else {
		logging.Infof(ctx, "Form factor not found: %v", formFactor)
	}
	switch features.FormFactor.FormFactor {
	case configpb.HardwareFeatures_FormFactor_CHROMEBASE, configpb.HardwareFeatures_FormFactor_CHROMEBIT, configpb.HardwareFeatures_FormFactor_CHROMEBOX, configpb.HardwareFeatures_FormFactor_CHROMESLATE:
		features.Keyboard.KeyboardType = configpb.HardwareFeatures_Keyboard_NONE
		features.Touchpad.TouchpadType = configpb.HardwareFeatures_Touchpad_NONE
	case configpb.HardwareFeatures_FormFactor_CLAMSHELL, configpb.HardwareFeatures_FormFactor_CONVERTIBLE:
		features.Keyboard.KeyboardType = configpb.HardwareFeatures_Keyboard_INTERNAL
		features.Touchpad.TouchpadType = configpb.HardwareFeatures_Touchpad_INTERNAL
	case configpb.HardwareFeatures_FormFactor_DETACHABLE:
		// When the dut is a detachable, check whether hammer exists
		// to determine if a removable keyboard is connected.
		hasHammer := checkHammer()
		if hasHammer {
			features.Keyboard.KeyboardType = configpb.HardwareFeatures_Keyboard_DETACHABLE
			features.Touchpad.TouchpadType = configpb.HardwareFeatures_Touchpad_DETACHABLE
		} else {
			features.Keyboard.KeyboardType = configpb.HardwareFeatures_Keyboard_NONE
			features.Touchpad.TouchpadType = configpb.HardwareFeatures_Touchpad_NONE
		}
	}

	keyboardBacklight, err := func() (bool, error) {
		out, err := crosConfig("/keyboard", "backlight")
		if err != nil {
			return false, err
		}
		return out == "true", nil
	}()
	if err != nil {
		logging.Infof(ctx, "Unknown /keyboard/backlight: %v", err)
	}

	if keyboardBacklight {
		features.Keyboard.Backlight = configpb.HardwareFeatures_PRESENT
	}

	checkForConnector := func(connectorRegexp string) bool {
		const drmSysFS = "/sys/class/drm"

		drmFiles, err := ioutil.ReadDir(drmSysFS)
		if err != nil {
			return false
		}

		cardMatch := regexp.MustCompile(connectorRegexp)
		for _, file := range drmFiles {
			fileName := file.Name()

			if cardMatch.MatchString(fileName) {
				if cardConnected, err := ioutil.ReadFile(path.Join(drmSysFS, fileName, "status")); err != nil {
					if !os.IsNotExist(err) {
						return false
					}
				} else {
					if strings.HasPrefix(string(cardConnected), "connected") {
						return true
					}
				}
			}
		}

		// No indication of internal panel connected and recognised.
		return false
	}

	// eDP displays show up as card*-eDP-1
	// MIPI panels show up as card*-DSI-1
	// Virtual displays in VMs show up as card*-Virtual-1
	internalDisplayRegexp := `^card[0-9]-(eDP|DSI|Virtual)-1$`
	hasInternalDisplay := checkForConnector(internalDisplayRegexp)
	if hasInternalDisplay {
		features.Screen.PanelProperties = &configpb.Component_DisplayPanel_Properties{}
	}

	// Display ports show up as card*-DP-[0-9]
	// HDMI ports show up as card*-HDMI-A-1
	// DVI ports show up as card*-DVI-I-1
	externalDisplayRegexp := `^card[0-9]-(DP|HDMI-A|DVI-I)-[0-9]$`
	hasExternalDisplay := checkForConnector(externalDisplayRegexp)
	switch {
	case hasInternalDisplay && hasExternalDisplay:
		features.Display.Type = configpb.HardwareFeatures_Display_TYPE_INTERNAL_EXTERNAL
	case hasExternalDisplay:
		features.Display.Type = configpb.HardwareFeatures_Display_TYPE_EXTERNAL
	case hasInternalDisplay:
		features.Display.Type = configpb.HardwareFeatures_Display_TYPE_INTERNAL
	}

	isHdmiConnected := func() bool {
		// HDMI ports show up as card*-HDMI-A-1
		hdmiCardRegex := `^card[0-9]-HDMI-A-[0-9]$`
		return checkForConnector(hdmiCardRegex)
	}()
	if isHdmiConnected {
		features.Hdmi.Present = configpb.HardwareFeatures_PRESENT
	} else {
		features.Hdmi.Present = configpb.HardwareFeatures_NOT_PRESENT
	}

	hasTouchScreen := func() bool {
		b, err := exec.Command("udevadm", "info", "--export-db").Output()
		if err != nil {
			return false
		}
		return regexp.MustCompile(`(?m)^E: ID_INPUT_TOUCHSCREEN=1$`).Match(b)
	}()
	if hasTouchScreen {
		features.Screen.TouchSupport = configpb.HardwareFeatures_PRESENT
	}

	hasTouchpad := func() bool {
		tp, err := exec.Command("udevadm", "info", "--export-db").Output()
		if err != nil {
			return false
		}
		return regexp.MustCompile(`(?m)^E: ID_INPUT_TOUCHPAD=1$`).Match(tp)
	}()
	if hasTouchpad {
		features.Touchpad.Present = configpb.HardwareFeatures_PRESENT
	} else {
		features.Touchpad.Present = configpb.HardwareFeatures_PRESENT_UNKNOWN
		features.Touchpad.TouchpadType = configpb.HardwareFeatures_Touchpad_TYPE_UNKNOWN
	}

	hasFingerprint := func() bool {
		out, err := crosConfig("/fingerprint", "sensor-location")
		if err != nil {
			return false
		}
		if out == "" || out == "none" {
			return false
		}
		return true
	}()
	features.Fingerprint.Present = hasFingerprint

	fingerprintBoard := func() string {
		out, err := crosConfig("/fingerprint", "board")
		if err != nil {
			return ""
		}
		return out
	}()
	features.Fingerprint.Board = fingerprintBoard

	features.Fingerprint.FingerprintDiag = &configpb.HardwareFeatures_Fingerprint_FingerprintDiag{}

	fingerprintDiagRoutineEnabled := func() bool {
		out, err := crosConfig("/cros-healthd/routines/fingerprint-diag", "routine-enable")
		return err == nil && out == "true"
	}()
	features.Fingerprint.FingerprintDiag.RoutineEnable = fingerprintDiagRoutineEnabled

	features.FwConfig.FwRwVersion = &configpb.HardwareFeatures_FirmwareConfiguration_SemVer{}
	if err := func() error {
		out, err := exec.Command("crossystem", "fwid").Output()
		if err != nil {
			return err
		}
		rwFwIds := strings.Split(string(out), ".")
		wantRwIds := rwFwIds[1:]
		versions := []*uint32{
			&features.FwConfig.FwRwVersion.MajorVersion,
			&features.FwConfig.FwRwVersion.MinorVersion,
			&features.FwConfig.FwRwVersion.PatchVersion,
		}
		if len(versions) != len(wantRwIds) {
			return errors.Errorf("got unexpected length of fwid: %s", out)
		}
		for idx, strID := range wantRwIds {
			id, err := strconv.Atoi(strID)
			if err != nil {
				return err
			}
			*versions[idx] = uint32(id)
		}
		return nil
	}(); err != nil {
		logging.Infof(ctx, "Unable to parse fwid versions: %v", err)
	}

	features.FwConfig.FwRoVersion = &configpb.HardwareFeatures_FirmwareConfiguration_SemVer{}
	if err := func() error {
		out, err := exec.Command("crossystem", "ro_fwid").Output()
		if err != nil {
			return err
		}
		roFwIds := strings.Split(string(out), ".")
		wantIds := roFwIds[1:]
		versions := []*uint32{
			&features.FwConfig.FwRoVersion.MajorVersion,
			&features.FwConfig.FwRoVersion.MinorVersion,
			&features.FwConfig.FwRoVersion.PatchVersion,
		}
		if len(versions) != len(wantIds) {
			return errors.Errorf("got unexpected length of ro_fwid: %s", out)
		}
		for idx, strID := range wantIds {
			// There are some fwids that look like Google_Foob.11297.437.0~RO
			numericPart := strings.Split(strID, "~")
			id, err := strconv.Atoi(numericPart[0])
			if err != nil {
				return err
			}
			*versions[idx] = uint32(id)
		}
		return nil
	}(); err != nil {
		logging.Infof(ctx, "Unable to parse ro_fwid versions: %v", err)
	}

	// Device has ChromeEC if /dev/cros_ec exists.
	// TODO(b/173741162): Pull EmbeddedController data directly from Boxster.
	if _, err := os.Stat("/dev/cros_ec"); err == nil {
		features.EmbeddedController.Present = configpb.HardwareFeatures_PRESENT
		features.EmbeddedController.EcType = configpb.HardwareFeatures_EmbeddedController_EC_CHROME
		// Check for features inferred from ectool inventory.
		output, err := exec.Command("ectool", "inventory").Output()
		if err != nil {
			features.EmbeddedController.FeatureTypecCmd = configpb.HardwareFeatures_PRESENT_UNKNOWN
			features.EmbeddedController.FeatureSystemSafeMode = configpb.HardwareFeatures_PRESENT_UNKNOWN
			features.EmbeddedController.FeatureAssertsPanic = configpb.HardwareFeatures_PRESENT_UNKNOWN
			features.EmbeddedController.FeatureMemoryDumpCommands = configpb.HardwareFeatures_PRESENT_UNKNOWN
		} else {
			// The presence of the integer "41" in the inventory output is a sufficient check, since 41 is
			// the bit position associated with this feature.
			if bytes.Contains(output, []byte("41")) {
				features.EmbeddedController.FeatureTypecCmd = configpb.HardwareFeatures_PRESENT
			} else {
				features.EmbeddedController.FeatureTypecCmd = configpb.HardwareFeatures_NOT_PRESENT
			}
			// EC_FEATURE_SYSTEM_SAFE_MODE == 47
			if bytes.Contains(output, []byte("47")) {
				features.EmbeddedController.FeatureSystemSafeMode = configpb.HardwareFeatures_PRESENT
			} else {
				features.EmbeddedController.FeatureSystemSafeMode = configpb.HardwareFeatures_NOT_PRESENT
			}
			// EC_FEATURE_ASSERT_REBOOTS == 48
			if bytes.Contains(output, []byte("48")) {
				features.EmbeddedController.FeatureAssertsPanic = configpb.HardwareFeatures_PRESENT
			} else {
				features.EmbeddedController.FeatureAssertsPanic = configpb.HardwareFeatures_NOT_PRESENT
			}
			// EC_FEATURE_ASSERT_REBOOTS == 51
			if bytes.Contains(output, []byte("51")) {
				features.EmbeddedController.FeatureMemoryDumpCommands = configpb.HardwareFeatures_PRESENT
			} else {
				features.EmbeddedController.FeatureMemoryDumpCommands = configpb.HardwareFeatures_NOT_PRESENT
			}
		}
		// Check if the detachable base is attached.
		output, err = exec.Command("ectool", "mkbpget", "switches").Output()
		if err != nil {
			features.EmbeddedController.DetachableBase = configpb.HardwareFeatures_PRESENT_UNKNOWN
		} else if strings.Contains(string(output), "Base attached: ON") {
			features.EmbeddedController.DetachableBase = configpb.HardwareFeatures_PRESENT
		} else {
			features.EmbeddedController.DetachableBase = configpb.HardwareFeatures_NOT_PRESENT
		}
		// Running `ectool chargecontrol` with no args will fail if version 2 isn't
		// supported. Check for battery sustainer output if the command doesn't
		// fail to make sure charger control v2 is fully supported.
		if out, err := exec.Command("ectool", "chargecontrol").Output(); err != nil || !regexp.MustCompile(`.*Battery sustainer`).Match(out) {
			logging.Infof(ctx, "Charge control V2 not supported: %v", err)
			features.EmbeddedController.FeatureChargeControlV2 = configpb.HardwareFeatures_NOT_PRESENT
		} else {
			features.EmbeddedController.FeatureChargeControlV2 = configpb.HardwareFeatures_PRESENT
		}
	}

	// Device has CBI if ectool cbi get doesn't raise error.
	if out, err := exec.Command("ectool", "cbi", "get", "0").Output(); err != nil {
		logging.Infof(ctx, "CBI not present: %v", err)
		features.EmbeddedController.Cbi = configpb.HardwareFeatures_NOT_PRESENT
	} else if strings.Contains(string(out), "As uint:") {
		features.EmbeddedController.Cbi = configpb.HardwareFeatures_PRESENT
	} else {
		features.EmbeddedController.Cbi = configpb.HardwareFeatures_PRESENT_UNKNOWN
	}

	getSuspendMode := func(ctx context.Context) (string, error) {
		if err := exec.CommandContext(ctx, "check_powerd_config", "--suspend_to_idle").Run(); err == nil {
			return "s2idle", nil
		}
		// Read the value from the power_manager suspend_mode configuration.
		out, err := os.ReadFile("/var/lib/power_manager/suspend_mode")
		if err != nil {
			out2, err2 := os.ReadFile("/usr/share/power_manager/suspend_mode")
			if err2 != nil {
				return "", errors.Wrap(err, err2.Error())
			}
			out = out2
		}
		// kernel_default is using the /sys/power/mem_sleep kernel configuration.
		if string(out) == "kernel_default" {
			out, err := os.ReadFile("/sys/power/mem_sleep")
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(string(out)), nil
		}
		return strings.TrimSpace(string(out)), nil
	}

	if mode, err := getSuspendMode(ctx); err != nil {
		logging.Info(ctx, "Failed to get suspend mode: ", err)
	} else if mode == "deep" {
		features.Suspend.SuspendToMem = configpb.HardwareFeatures_PRESENT
	} else if mode == "s2idle" {
		features.Suspend.SuspendToIdle = configpb.HardwareFeatures_PRESENT
	} else {
		logging.Info(ctx, "Unrecognized suspend configuration: ", mode)
	}

	// Get number of PD ports from ectool, if possible.
	if out, err := exec.Command("ectool", "usbpdpower").Output(); err == nil {
		// Returns multiple lines like:
		// Port 0: SNK Charger PD 15263mV / 3000mA, max 15000mV / 3000mA / 45000mW
		// Port 1: Disconnected
		// We want to match the last port number seen to get the count.
		re := regexp.MustCompile(`Port (\d+):`)
		match := re.FindAllSubmatch(out, -1)
		if match == nil {
			// If ectool succeeds but doesn't match the regex something is very wrong. I.e. someone modified the output of ectool.
			return nil, errors.Errorf("ectool usbpdpower output unexpected: %v", string(out))
		}
		count, err := strconv.Atoi(string(match[len(match)-1][1]))
		if err != nil {
			return nil, errors.Errorf("Failed to parse: %v", string(match[len(match)-1][1]))
		}
		features.UsbC = &configpb.HardwareFeatures_UsbC{Count: &configpb.HardwareFeatures_Count{Value: uint32(count + 1)}}
	}

	// Device has GSC with production RW KeyId if gsctool -a -I -M
	// returns RW KeyID with value 0x87b73b67 or 0xde88588d
	func() {
		if out, err := exec.Command("gsctool", "-a", "-f", "-M").Output(); err != nil {
			logging.Infof(ctx, "Failed to exec command for KeyId info: %v", err)
			features.TrustedPlatformModule.ProductionRwKeyId = configpb.HardwareFeatures_PRESENT_UNKNOWN
		} else if keyIDRW, err := findGSCKeyID(string(out), "RW"); err != nil {
			logging.Infof(ctx, "Failed to read RW KeyId: %v", err)
		} else if containsGSCKeyID(prodRWGSCKeyIDs, GSCKeyID(keyIDRW)) {
			features.TrustedPlatformModule.ProductionRwKeyId = configpb.HardwareFeatures_PRESENT
		} else {
			features.TrustedPlatformModule.ProductionRwKeyId = configpb.HardwareFeatures_NOT_PRESENT
		}
	}()

	// Whether device has TPM enabled can be checked by `tpm_manager_client status`.
	// If TPM is enabled, we can check the version by `tpm_version`.
	func() {
		features.TrustedPlatformModule.RuntimeTpmVersion = configpb.HardwareFeatures_TrustedPlatformModule_TPM_VERSION_DISABLED
		out, err := exec.Command("tpm_manager_client", "status", "--nonsensitive").Output()
		if err != nil {
			logging.Info(ctx, "Failed to exec command `tpm_manager_client status`: ", err)
			return
		}
		if !strings.Contains(string(out), "is_enabled: true") {
			return
		}
		out, err = exec.Command("tpm_version").Output()
		if err != nil {
			logging.Info(ctx, "Failed to exec command `tpm_version`: ", err)
			return
		}
		status := string(out)
		if strings.Contains(status, "TPM 1.2") {
			features.TrustedPlatformModule.RuntimeTpmVersion = configpb.HardwareFeatures_TrustedPlatformModule_TPM_VERSION_V1_2
		} else if strings.Contains(status, "TPM 2.0") {
			features.TrustedPlatformModule.RuntimeTpmVersion = configpb.HardwareFeatures_TrustedPlatformModule_TPM_VERSION_V2
		}
	}()

	// Check if rollback NVRAM space is present in the TPM.
	func() {
		out, err := exec.Command("tpm_manager_client", "list_spaces").Output()
		if err != nil {
			logging.Info(ctx, "Failed to exec command `tpm_manager_client list_spaces`: ", err)
			features.TrustedPlatformModule.EnterpriseRollbackSpace = configpb.HardwareFeatures_PRESENT_UNKNOWN
			return
		}
		if bytes.Contains(out, []byte("0x0000100E")) {
			features.TrustedPlatformModule.EnterpriseRollbackSpace = configpb.HardwareFeatures_PRESENT
			return
		}
		features.TrustedPlatformModule.EnterpriseRollbackSpace = configpb.HardwareFeatures_NOT_PRESENT
	}()

	// Obtain GSC name from existence of /opt/google/{cr50,ti50}/firmware directory.
	func() {
		if _, err := os.Stat("/opt/google/cr50/firmware"); err == nil {
			features.TrustedPlatformModule.GscFwName = configpb.HardwareFeatures_TrustedPlatformModule_GSC_CR50
		} else if _, err := os.Stat("/opt/google/ti50/firmware"); err == nil {
			features.TrustedPlatformModule.GscFwName = configpb.HardwareFeatures_TrustedPlatformModule_GSC_TI50
		} else {
			features.TrustedPlatformModule.GscFwName = configpb.HardwareFeatures_TrustedPlatformModule_GSC_NONE
		}
	}()

	modemVariant, err := crosConfig("/modem", "firmware-variant")
	features.Cellular.Present = configpb.HardwareFeatures_NOT_PRESENT
	if err != nil {
		logging.Infof(ctx, "Modem not found with err: %v", err)
	} else if modemVariant == "" {
		logging.Info(ctx, "Modem not found")
	} else {
		features.Cellular.Present = configpb.HardwareFeatures_PRESENT
		features.Cellular.Model = modemVariant
		swDynamicSar, err := func() (bool, error) {
			out, err := crosConfig("/power", "use-modemmanager-for-dynamic-sar")
			if err != nil {
				return false, err
			}
			return out == "1", nil
		}()
		if err != nil {
			logging.Infof(ctx, "Unknown /power/use-modemmanager-for-dynamic-sar: %v", err)
		}
		features.Cellular.DynamicPowerReductionConfig = &configpb.HardwareFeatures_Cellular_DynamicPowerReductionConfig{
			DynamicPowerReductionConfig: &configpb.HardwareFeatures_Cellular_DynamicPowerReductionConfig_ModemManager{ModemManager: swDynamicSar}}
	}

	// bluetoothctl hangs when bluetoothd is not built with asan enabled or
	// crashes. Set state to PRESENT_UNKNOWN on timeout.
	const timeout = 3 * time.Second
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if out, err := exec.CommandContext(cmdCtx, "bluetoothctl", "list").Output(); err != nil {
		features.Bluetooth.Present = configpb.HardwareFeatures_PRESENT_UNKNOWN
	} else if len(string(out)) != 0 {
		features.Bluetooth.Present = configpb.HardwareFeatures_PRESENT
	} else {
		logging.Infof(ctx, "bluetooth controller not found")
		features.Bluetooth.Present = configpb.HardwareFeatures_NOT_PRESENT
	}

	readSysfsString := func(dev, relPath string) string {
		path := "/sys/block/" + dev + "/" + relPath
		out, err := exec.CommandContext(cmdCtx, "cat", path).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}

	hasEmmcStorage := func() bool {
		matches, err := filepath.Glob("/dev/mmc*")
		if err != nil {
			return false
		}
		for _, dev := range matches {
			pathComponenets := strings.Split(dev, "/")
			phyDev := pathComponenets[len(pathComponenets)-1]
			out := readSysfsString(phyDev, "device/type")
			if out == "MMC" {
				return true
			}
		}
		return false
	}()
	if hasEmmcStorage {
		features.Storage.StorageType = configpb.Component_Storage_EMMC
	}

	hasEmmcBridgeStorage := func() bool {
		const bh799Ven = 0x1217
		const bh799Dev = 0x0002
		var readSysfsHex func(string, string) int64
		readSysfsHex = func(devPath, relPath string) int64 {
			pathComponenets := strings.Split(devPath, "/")
			phyDev := pathComponenets[len(pathComponenets)-1]
			outStr := readSysfsString(phyDev, relPath)
			if outStr == "" {
				return 0
			}
			if outStr[0:2] == "0x" {
				outStr = outStr[2:]
			}
			val, err := strconv.ParseInt(outStr, 16, 64)
			if err != nil {
				return 0
			}
			return val
		}
		matches, err := filepath.Glob("/dev/nvme*")
		if err != nil {
			return false
		}
		for _, dev := range matches {
			subsysVen := readSysfsHex(dev, "device/device/subsystem_vendor")
			subsysDev := readSysfsHex(dev, "device/device/subsystem_device")
			if subsysVen == bh799Ven && subsysDev == bh799Dev {
				return true
			}
		}
		return false
	}()
	if hasEmmcBridgeStorage {
		features.Storage.StorageType = configpb.Component_Storage_BRIDGED_EMMC
	}

	hasNvmeStorage := func() bool {
		matches, err := filepath.Glob("/dev/nvme*")
		if err != nil {
			return false
		}
		if len(matches) > 0 {
			return true
		}
		return false
	}()
	if hasNvmeStorage && !hasEmmcBridgeStorage {
		features.Storage.StorageType = configpb.Component_Storage_NVME
	}

	hasNvmeSelfTestStorage := func() bool {
		matches, err := filepath.Glob("/dev/nvme*n1")
		if err != nil {
			return false
		}
		if len(matches) == 0 {
			return false
		}

		nvmePath := matches[0]
		b, err := exec.Command("nvme", "id-ctrl", "-H", nvmePath).Output()
		if err != nil {
			return false
		}
		return bytes.Contains(b, []byte("Device Self-test Supported"))
	}()
	if hasNvmeStorage && hasNvmeSelfTestStorage {
		config.HasNvmeSelfTest = true
	}

	hasSataStorage := func() bool {
		matches, err := filepath.Glob("/dev/sd*")
		if err != nil {
			return false
		}
		for _, dev := range matches {
			pathComponenets := strings.Split(dev, "/")
			physDev := pathComponenets[len(pathComponenets)-1]
			outStr := readSysfsString(physDev, "device/inquiry")
			if strings.Contains(outStr, "ATA") {
				return true
			}
			return false
		}
		return false
	}()
	if hasSataStorage {
		features.Storage.StorageType = configpb.Component_Storage_SATA
	}

	hasUfsStorage := func() bool {
		matches, err := filepath.Glob("/dev/sd*")
		if err != nil {
			return false
		}
		for _, dev := range matches {
			pathComponenets := strings.Split(dev, "/")
			physDev := pathComponenets[len(pathComponenets)-1]
			// /sys/block/<dev>/device is a symlink to the scsi target node,
			// the symlink resolution will resolve .. directory against the linked one
			// so we can get to the controller's node
			path := "/sys/block/" + physDev + "/device/../../../driver"
			out, err := exec.Command("readlink", "-f", path).Output()
			if err != nil {
				return false
			}
			resComponents := strings.Split(strings.TrimSpace(string(out)), "/")
			res := resComponents[len(resComponents)-1]
			if res == "ufshcd" {
				return true
			}
			return false
		}
		return false
	}()
	if hasUfsStorage {
		features.Storage.StorageType = configpb.Component_Storage_UFS
	}

	func() {
		// This function determines DUT's power supply type and stores it to config.Power.
		// If DUT has a battery, config.Power is DeprecatedDeviceConfig_POWER_SUPPLY_BATTERY.
		// If DUT has AC power supplies only, config.Power is DeprecatedDeviceConfig_POWER_SUPPLY_AC_ONLY.
		// Otherwise, DeprecatedDeviceConfig_POWER_SUPPLY_UNSPECIFIED is populated.
		const sysFsPowerSupplyPath = "/sys/class/power_supply"
		// AC power types come from power_supply driver in Linux kernel (drivers/power/supply/power_supply_sysfs.c)
		acPowerTypes := [...]string{
			"Unknown", "UPS", "Mains", "USB",
			"USB_DCP", "USB_CDP", "USB_ACA", "USB_C",
			"USB_PD", "USB_PD_DRP", "BrickID",
		}
		isACPower := make(map[string]bool)
		for _, s := range acPowerTypes {
			isACPower[s] = true
		}
		config.Power = protocol.DeprecatedDeviceConfig_POWER_SUPPLY_UNSPECIFIED
		files, err := ioutil.ReadDir(sysFsPowerSupplyPath)
		if err != nil {
			logging.Infof(ctx, "Failed to read %v: %v", sysFsPowerSupplyPath, err)
			return
		}
		for _, file := range files {
			devPath := path.Join(sysFsPowerSupplyPath, file.Name())
			supplyTypeBytes, err := ioutil.ReadFile(path.Join(devPath, "type"))
			supplyType := strings.TrimSuffix(string(supplyTypeBytes), "\n")
			if err != nil {
				logging.Infof(ctx, "Failed to read supply type of %v: %v", devPath, err)
				continue
			}
			if strings.HasPrefix(supplyType, "Battery") {
				supplyScopeBytes, err := ioutil.ReadFile(path.Join(devPath, "scope"))
				supplyScope := strings.TrimSuffix(string(supplyScopeBytes), "\n")
				if err != nil && !os.IsNotExist(err) {
					// Ignore NotExist error since /sys/class/power_supply/*/scope may not exist
					logging.Infof(ctx, "Failed to read supply type of %v: %v", devPath, err)
					continue
				}
				if strings.HasPrefix(string(supplyScope), "Device") {
					// Ignore batteries for peripheral devices.
					continue
				}
				config.Power = protocol.DeprecatedDeviceConfig_POWER_SUPPLY_BATTERY
				// Found at least one battery so this device is powered by battery.
				break
			}
			if !isACPower[supplyType] {
				logging.Infof(ctx, "Unknown supply type %v for %v", supplyType, devPath)
				continue
			}
			config.Power = protocol.DeprecatedDeviceConfig_POWER_SUPPLY_AC_ONLY
		}
	}()

	storageBytes, err := func() (int64, error) {
		b, err := exec.Command("lsblk", "-J", "-b").Output()
		if err != nil {
			return 0, err
		}
		return findDiskSize(b)
	}()
	if err != nil {
		logging.Infof(ctx, "Failed to get disk size: %v", err)
	}
	features.Storage.SizeGb = uint32(storageBytes / 1_000_000_000)

	memoryBytes, err := func() (int64, error) {
		b, err := ioutil.ReadFile("/proc/meminfo")
		if err != nil {
			return 0, err
		}
		return findMemorySize(b)
	}()
	if err != nil {
		logging.Infof(ctx, "Failed to get memory size: %v", err)
	}
	features.Memory.Profile = &configpb.Component_Memory_Profile{
		// Use 2^20 megabytes to be consistent with how memory size is advertised.
		SizeMegabytes: int32(memoryBytes >> 20),
	}

	lidMicrophone, err := matchCrasDeviceType(`(INTERNAL|FRONT)_MIC`)
	if err != nil {
		logging.Infof(ctx, "Failed to get lid microphone: %v", err)
	}
	features.Audio.LidMicrophone = lidMicrophone
	baseMicrophone, err := matchCrasDeviceType(`REAR_MIC`)
	if err != nil {
		logging.Infof(ctx, "Failed to get base microphone: %v", err)
	}
	features.Audio.BaseMicrophone = baseMicrophone
	expectAudio := hasBuiltinAudio(ctx, features.FormFactor.FormFactor)
	if features.Audio.LidMicrophone.GetValue() == 0 && features.Audio.BaseMicrophone.GetValue() == 0 && expectAudio {
		features.Audio.LidMicrophone = &configpb.HardwareFeatures_Count{Value: 1}
	}
	speaker, err := matchCrasDeviceType(`INTERNAL_SPEAKER`)
	if err != nil {
		logging.Infof(ctx, "Failed to get speaker: %v", err)
	}

	if speaker.GetValue() > 0 || expectAudio {

		amp, err := findSpeakerAmplifier()
		if err != nil {
			logging.Infof(ctx, "Failed to get amp: %v", err)
		}
		if amp == nil {
			// Do not assume findSpeakerAmplifier() always returns a non-nil amp.
			// Always signal that the device has a hwdep.Speaker().
			amp = &configpb.Component_Amplifier{}
		}
		features.Audio.SpeakerAmplifier = amp
	}

	hasPrivacyScreen := func() bool {
		// Get list of connectors.
		value, err := exec.Command("modetest", "-c").Output()
		if err != nil {
			logging.Infof(ctx, "Failed to get connectors: %v", err)
			return false
		}

		modetestInfo := string(value)
		// Check if privacy-screen prop is present.
		if strings.Contains(modetestInfo, "privacy-screen sw-state:") && strings.Contains(modetestInfo, "privacy-screen hw-state:") {
			return true
		}

		// Fallback to legacy style.
		return strings.Contains(modetestInfo, "privacy-screen:")
	}()
	if hasPrivacyScreen {
		features.PrivacyScreen.Present = configpb.HardwareFeatures_PRESENT
	} else {
		features.PrivacyScreen.Present = configpb.HardwareFeatures_NOT_PRESENT
	}

	cpuSMT, err := func() (bool, error) {
		// NB: this sysfs API exists only on kernel >=4.19 (b/195061310). But we don't
		// target SMT-specific tests on earlier kernels.
		b, err := ioutil.ReadFile("/sys/devices/system/cpu/smt/control")
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, errors.Wrap(err, "failed to read SMT control file")
		}
		s := strings.TrimSpace(string(b))
		switch s {
		case "on", "off", "forceoff":
			return true, nil
		case "notsupported", "notimplemented":
			return false, nil
		default:
			return false, errors.Errorf("unknown SMT control status: %q", s)
		}
	}()
	if err != nil {
		logging.Infof(ctx, "Failed to determine CPU SMT features: %v", err)
	}
	if cpuSMT {
		features.Soc.Features = append(features.Soc.Features, configpb.Component_Soc_SMT)
	}

	cpuVulnerabilityPresent := func(vulnerability string) (bool, error) {
		vPath := filepath.Join("/sys/devices/system/cpu/vulnerabilities", vulnerability)
		b, err := ioutil.ReadFile(vPath)
		if err != nil {
			return false, errors.Wrapf(err, "failed to read vulnerability file %q", vPath)
		}
		s := strings.TrimSpace(string(b))
		if s == "Not affected" {
			return false, nil
		}
		return true, nil
	}

	cpuL1TF, err := cpuVulnerabilityPresent("l1tf")
	if err != nil {
		logging.Infof(ctx, "Failed to determine L1TF vulnerability: %v", err)
	} else if cpuL1TF {
		features.Soc.Vulnerabilities = append(features.Soc.Vulnerabilities, configpb.Component_Soc_L1TF)
	}

	cpuMDS, err := cpuVulnerabilityPresent("mds")
	if err != nil {
		logging.Infof(ctx, "Failed to determine MDS vulnerability: %v", err)
	} else if cpuMDS {
		features.Soc.Vulnerabilities = append(features.Soc.Vulnerabilities, configpb.Component_Soc_MDS)
	}

	for _, v := range info.flags {
		if v == "sha_ni" {
			features.Soc.Features = append(features.Soc.Features, configpb.Component_Soc_SHA_NI)
		}
	}

	func() {
		// Probe for presence of DisplayPort converters
		devices := map[string]string{
			"i2c-10EC2141:00": "RTD2141B",
			"i2c-10EC2142:00": "RTD2142",
			"i2c-1AF80175:00": "PS175",
		}
		for f, name := range devices {
			path := filepath.Join("/sys/bus/i2c/devices", f)
			if _, err := os.Stat(path); err != nil {
				continue
			}
			features.DpConverter.Converters = append(features.DpConverter.Converters, &configpb.Component_DisplayPortConverter{
				Name: name,
			})
		}
	}()

	hasHps, err := func() (bool, error) {
		out, err := crosConfig("/hps", "has-hps")
		if err != nil {
			return false, err
		}
		return out == "true", nil
	}()
	if err != nil {
		logging.Infof(ctx, "Unknown /hps: %v", err)
	}
	if hasHps {
		features.Hps.Present = configpb.HardwareFeatures_PRESENT
	}

	camFeatures, err := cameraFeatures(model)
	if err != nil {
		logging.Infof(ctx, "failed to load camera feature profile: %v", err)
	}
	features.Camera.Features = camFeatures
	camEnumerated, camEnumeratedUsbIds, err := cameraEnumerated()
	if err != nil {
		logging.Infof(ctx, "failed to load camera enumeration status: %v", err)
	}
	features.Camera.Enumerated = camEnumerated
	features.Camera.EnumeratedUsbIds = camEnumeratedUsbIds

	if err := parseKConfigs(ctx, features); err != nil {
		logging.Info(ctx, "Failed to parse BIOS kConfig: ", err)
	}

	if err := parseECBuildConfig(ctx, features); err != nil {
		logging.Info(ctx, "Failed to parse EC Config: ", err)
	}

	rpConfigPresent, err := hasRuntimeProbeConfig(model, false)
	if err != nil {
		logging.Info(ctx, "Failed to determine if the config of Runtime Probe exists: ", err)
	}
	if rpConfigPresent {
		features.RuntimeProbeConfig.Present = configpb.HardwareFeatures_PRESENT
	} else {
		logging.Infof(ctx, "Config of Runtime Probe not found")
		features.RuntimeProbeConfig.Present = configpb.HardwareFeatures_NOT_PRESENT
	}

	rpEncryptedConfigPresent, err := hasRuntimeProbeConfig(model, true)
	if err != nil {
		logging.Info(ctx, "Failed to determine if the encrypted config of Runtime Probe exists: ", err)
	}
	if rpEncryptedConfigPresent {
		features.RuntimeProbeConfig.EncryptedConfigPresent = configpb.HardwareFeatures_PRESENT
	} else {
		logging.Infof(ctx, "Encrypted config of Runtime Probe not found")
		features.RuntimeProbeConfig.EncryptedConfigPresent = configpb.HardwareFeatures_NOT_PRESENT
	}

	gpuFamily, gpuVendor, cpuSocFamily, err := func() (gpuFamily, gpuVendor, cpuSocFamily string, fetchErr error) {
		outBin, err := ioutil.TempFile("/tmp", "hardware_probe")
		if err != nil {
			return "", "", "", errors.Wrap(err, "failed to create temp file")
		}
		outBin.Close()
		defer os.Remove(outBin.Name())
		out, err := exec.Command("/usr/local/graphics/hardware_probe", "-output", outBin.Name()).CombinedOutput()
		if err != nil {
			return "", "", "", errors.Wrapf(err, "failed to run hardware_probe, output: %v", string(out))
		}
		type gpuInfo struct {
			Family string `json:"Family"`
			Vendor string `json:"GPUVendor"`
		}

		type hardwareProbeResult struct {
			GPUInfo   []gpuInfo `json:"GPU_Family"`
			CPUFamily string    `json:"CPU_SOC_Family"`
		}
		b, err := os.ReadFile(outBin.Name())
		if err != nil {
			return "", "", "", errors.Wrap(err, "failed to read hardware_probe.json")
		}
		var result hardwareProbeResult
		if err := json.Unmarshal(b, &result); err != nil {
			return "", "", "", err
		}
		if len(result.GPUInfo) > 0 {
			gpuFamily = result.GPUInfo[0].Family
			gpuVendor = result.GPUInfo[0].Vendor
			if len(result.GPUInfo) > 1 {
				logging.Infof(ctx, "Found multiple GPUInfo(%v), only use the first one detected.", result.GPUInfo)
			}
		}
		cpuSocFamily = result.CPUFamily
		return
	}()
	if err != nil {
		logging.Infof(ctx, "failed to parse hardware_probe output: %v", err)
	}
	features.HardwareProbeConfig.GpuFamily = gpuFamily
	features.HardwareProbeConfig.GpuVendor = gpuVendor
	features.HardwareProbeConfig.CpuSocFamily = cpuSocFamily

	hevcSupport, err := func() (configpb.HardwareFeatures_Present, error) {
		out, err := crosConfig("/ui", "serialized-ash-switches")
		if err != nil {
			return configpb.HardwareFeatures_PRESENT_UNKNOWN, err
		}
		if regexp.MustCompile("disable-features=[^\x00]*PlatformHEVCDecoderSupport").MatchString(out) {
			return configpb.HardwareFeatures_NOT_PRESENT, nil
		}
		if regexp.MustCompile("enable-features=[^\x00]*PlatformHEVCDecoderSupport").MatchString(out) {
			return configpb.HardwareFeatures_PRESENT, nil
		}
		return configpb.HardwareFeatures_PRESENT_UNKNOWN, nil
	}()
	if err != nil {
		logging.Infof(ctx, "Unknown /ui/serialized-ash-switches: %v", err)
	}
	features.Soc.HevcSupport = hevcSupport

	hasVrrSupport := func() bool {
		modetestOutput, err := exec.Command("modetest").Output()
		if err != nil {
			logging.Infof(ctx, "failed to run modetest: %v", err)
			return false
		}
		// check if there exists a vrr_capable property with a value set to 1 in
		// the modetest command output
		regex := regexp.MustCompile("\n\t.*vrr_capable:(\n\t\t.*)*\n\t\tvalue: 1")
		return regex.Match(modetestOutput)
	}()
	if hasVrrSupport {
		features.Vrr.Present = configpb.HardwareFeatures_PRESENT
	} else {
		features.Vrr.Present = configpb.HardwareFeatures_NOT_PRESENT
	}

	hasTiledDisplay := func() bool {
		modetestOutput, err := exec.Command("modetest", "-c").Output()
		if err != nil {
			logging.Infof(ctx, "failed to run modetest: %v", err)
			return false
		}
		// Check if there exists a TILE property with a non-empty value set in the modetest output.
		// Value, if present, is printed on a new line with an extra tab.
		regex := regexp.MustCompile("\n\t.*TILE:(\n\t\t*.*){0,4}\n\n\t\tvalue:(\n\t\t\t.*[^:][0-9a-z].*)")
		return regex.Match(modetestOutput)
	}()
	if hasTiledDisplay {
		features.TiledDisplay.Present = configpb.HardwareFeatures_PRESENT
	} else {
		features.TiledDisplay.Present = configpb.HardwareFeatures_NOT_PRESENT
	}

	features.FeatureLevel, err = featureLevel()
	if err != nil {
		logging.Infof(ctx, "Failed to get feature level: %v", err)
	}

	features.OemInfo = &configpb.HardwareFeatures_OEMInfo{
		Name: oemName(),
	}

	// FirmwareFeatures
	altfw, err := func() (bool, error) {
		out, err := crosConfig("/firmware", "has-alt-firmware")
		if err != nil {
			return false, err
		}
		return out == "true", nil
	}()
	if err != nil {
		logging.Infof(ctx, "Unknown firmware/altfw-present : %v", err)
	}
	if altfw {
		swConfig.FirmwareInfo.HasAltFirmware = true
	} else {
		swConfig.FirmwareInfo.HasAltFirmware = false
	}

	// InterruptControllerInfo

	// Default to assuming we have NMI support. We only say NMI is
	// unsupported if we've explicitly decided we can't support it for
	// a given configuration. Note that upon any errors we'll leave this
	// as "present". If a test doesn't care then that's fine. If a test
	// relies on NMI support then we _want_ the test to fail and point out
	// that we failed to get the config properly.
	features.InterruptControllerInfo.NmiSupport = configpb.HardwareFeatures_PRESENT
	if runtime.GOARCH == "arm" {
		features.InterruptControllerInfo.NmiSupport = configpb.HardwareFeatures_NOT_PRESENT
	} else if runtime.GOARCH == "arm64" {
		// GICv2 can't support NMIs. There's no wonderful way to detect
		// the version of the GIC, but /proc/interrupts will contain
		// GICv2 in this case so we'll use that.
		if interruptsOutput, err := os.ReadFile("/proc/interrupts"); err != nil {
			// Give out a warning because /proc/interrupts should always exist.
			logging.Infof(ctx, "Couldn't read /proc/interrupts: %v", err)
		} else if strings.Contains(string(interruptsOutput), "GICv2") {
			features.InterruptControllerInfo.NmiSupport = configpb.HardwareFeatures_NOT_PRESENT
		}

		// If the device tree indicates broken firmware then we know we
		// don't support NMI.
		if brokenGicOutput, err := exec.Command("find", "/proc/device-tree/", "-name", "mediatek,broken-save-restore-fw").Output(); err != nil {
			// All arm64 should have /proc/device-tree/
			logging.Infof(ctx, "Failed to search device-tree for GIC FW Quirk: %v", err)
		} else if string(brokenGicOutput) != "" {
			features.InterruptControllerInfo.NmiSupport = configpb.HardwareFeatures_NOT_PRESENT
		}
	}

	features.Accelerometer = &configpb.HardwareFeatures_Accelerometer{}
	func() {
		out, err := crosConfig("/hardware-properties", "has-base-accelerometer")
		if err != nil {
			features.Accelerometer.BaseAccelerometer = configpb.HardwareFeatures_PRESENT_UNKNOWN
			return
		}
		if out == "" || out == "false" {
			features.Accelerometer.BaseAccelerometer = configpb.HardwareFeatures_NOT_PRESENT
			return
		}
		features.Accelerometer.BaseAccelerometer = configpb.HardwareFeatures_PRESENT
	}()

	func() {
		out, err := exec.CommandContext(cmdCtx, "lspci", "-mm").Output()
		if err != nil {
			features.FwConfig.IntelIsh = configpb.HardwareFeatures_PRESENT_UNKNOWN
			return
		}
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Integrated Sensor Hub") {
				features.FwConfig.IntelIsh = configpb.HardwareFeatures_PRESENT
				return
			}
		}
		features.FwConfig.IntelIsh = configpb.HardwareFeatures_NOT_PRESENT
	}()

	return &protocol.HardwareFeatures{
		HardwareFeatures:       features,
		DeprecatedDeviceConfig: config,
		SoftwareConfig:         swConfig,
	}, nil
}

type lscpuEntry struct {
	Field string `json:"field"` // includes trailing ":"
	Data  string `json:"data"`
}

type lscpuResult struct {
	Entries []lscpuEntry `json:"lscpu"`
}

func (r *lscpuResult) find(name string) (data string, ok bool) {
	for _, e := range r.Entries {
		if e.Field == name {
			return e.Data, true
		}
	}
	return "", false
}

type cpuConfig struct {
	cpuArch protocol.DeprecatedDeviceConfig_Architecture
	soc     protocol.DeprecatedDeviceConfig_SOC
	flags   []string
}

// cpuInfo returns a structure containing field data from the "lscpu" command
// which outputs CPU architecture information from "sysfs" and "/proc/cpuinfo".
func cpuInfo() (cpuConfig, error) {
	errInfo := cpuConfig{protocol.DeprecatedDeviceConfig_ARCHITECTURE_UNDEFINED, protocol.DeprecatedDeviceConfig_SOC_UNSPECIFIED, nil}
	b, err := exec.Command("lscpu", "--json").Output()
	if err != nil {
		return errInfo, err
	}
	var parsed lscpuResult
	if err := json.Unmarshal(b, &parsed); err != nil {
		return errInfo, errors.Wrap(err, "failed to parse lscpu result")
	}
	flagsStr, _ := parsed.find("Flags:")
	flags := strings.Split(flagsStr, " ")
	arch, err := findArchitecture(parsed)
	if err != nil {
		return errInfo, errors.Wrap(err, "failed to find CPU architecture")
	}
	soc, err := findSOC(parsed)
	if err != nil {
		return cpuConfig{arch, protocol.DeprecatedDeviceConfig_SOC_UNSPECIFIED, flags}, errors.Wrap(err, "failed to find SOC")
	}
	return cpuConfig{arch, soc, flags}, nil
}

// findArchitecture returns an architecture configuration based from parsed output
// data value of the "Architecture" field.
func findArchitecture(parsed lscpuResult) (protocol.DeprecatedDeviceConfig_Architecture, error) {
	arch, ok := parsed.find("Architecture:")
	if !ok {
		return protocol.DeprecatedDeviceConfig_ARCHITECTURE_UNDEFINED, errors.New("failed to find Architecture field")
	}

	switch arch {
	case "x86_64":
		return protocol.DeprecatedDeviceConfig_X86_64, nil
	case "i686":
		return protocol.DeprecatedDeviceConfig_X86, nil
	case "aarch64":
		return protocol.DeprecatedDeviceConfig_ARM64, nil
	case "armv7l", "armv8l":
		return protocol.DeprecatedDeviceConfig_ARM, nil
	default:
		return protocol.DeprecatedDeviceConfig_ARCHITECTURE_UNDEFINED, errors.Errorf("unknown architecture: %q", arch)
	}
}

// findSOC returns a SOC configuration based from parsed output data value of the
// "Vendor ID" and other related fields.
func findSOC(parsed lscpuResult) (protocol.DeprecatedDeviceConfig_SOC, error) {
	vendorID, ok := parsed.find("Vendor ID:")
	if !ok {
		return protocol.DeprecatedDeviceConfig_SOC_UNSPECIFIED, errors.New("failed to find Vendor ID field")
	}

	switch vendorID {
	case "ARM":
		fallthrough
	case "Qualcomm":
		return findARMSOC()
	case "GenuineIntel":
		return findIntelSOC(&parsed)
	case "AuthenticAMD":
		return findAMDSOC(&parsed)
	default:
		return protocol.DeprecatedDeviceConfig_SOC_UNSPECIFIED, errors.Errorf("unknown vendor ID: %q", vendorID)
	}
}

// findARMSOC returns an ARM SOC configuration based on "soc_id" from "/sys/bus/soc/devices".
func findARMSOC() (protocol.DeprecatedDeviceConfig_SOC, error) {
	// Platforms with SMCCC >= 1.2 should implement get_soc functions in firmware
	const socSysFS = "/sys/bus/soc/devices"
	socs, err := ioutil.ReadDir(socSysFS)
	if err == nil {
		for _, soc := range socs {
			c, err := ioutil.ReadFile(path.Join(socSysFS, soc.Name(), "soc_id"))
			if err != nil || !strings.HasPrefix(string(c), "jep106:") {
				continue
			}
			// Trim trailing \x00 and \n
			socID := strings.TrimRight(string(c), "\x00\n")
			switch socID {
			case "jep106:0070:01a9":
				fallthrough
			case "jep106:0070:01ef":
				fallthrough
			case "jep106:0070:7180": // Used by older SC7180 firmware
				return protocol.DeprecatedDeviceConfig_SOC_SC7180, nil
			case "jep106:0070:7280":
				return protocol.DeprecatedDeviceConfig_SOC_SC7280, nil
			case "jep106:0426:8192":
				return protocol.DeprecatedDeviceConfig_SOC_MT8192, nil
			case "jep106:0426:8186":
				return protocol.DeprecatedDeviceConfig_SOC_MT8186, nil
			case "jep106:0426:8195":
				return protocol.DeprecatedDeviceConfig_SOC_MT8195, nil
			case "jep106:0426:8188":
				return protocol.DeprecatedDeviceConfig_SOC_MT8188G, nil
			case "jep106:0426:8196":
				return protocol.DeprecatedDeviceConfig_SOC_MT8196, nil
			default:
				return protocol.DeprecatedDeviceConfig_SOC_UNSPECIFIED, errors.Errorf("unknown ARM model: %s", socID)
			}
		}
	}

	// For old platforms with SMCCC < 1.2: mt8173, mt8183, rk3288, rk3399,
	// match with their compatible string. Obtain the string after the last , and trim \x00.
	// Example: google,krane-sku176\x00google,krane\x00mediatek,mt8183\x00
	c, err := ioutil.ReadFile("/sys/firmware/devicetree/base/compatible")
	if err != nil {
		return protocol.DeprecatedDeviceConfig_SOC_UNSPECIFIED, errors.Wrap(err, "failed to find ARM model")
	}

	compatible := string(c)
	model := strings.ToLower(compatible[strings.LastIndex(compatible, ",")+1:])
	model = strings.TrimRight(model, "\x00")

	switch model {
	case "mt8173":
		return protocol.DeprecatedDeviceConfig_SOC_MT8173, nil
	case "mt8183":
		return protocol.DeprecatedDeviceConfig_SOC_MT8183, nil
	case "rk3288":
		return protocol.DeprecatedDeviceConfig_SOC_RK3288, nil
	case "rk3399":
		return protocol.DeprecatedDeviceConfig_SOC_RK3399, nil
	default:
		return protocol.DeprecatedDeviceConfig_SOC_UNSPECIFIED, errors.Errorf("unknown ARM model: %s", model)
	}
}

// findIntelSOC returns an Intel SOC configuration based on "CPU family", "Model",
// and "Model name" fields.
func findIntelSOC(parsed *lscpuResult) (protocol.DeprecatedDeviceConfig_SOC, error) {
	if family, ok := parsed.find("CPU family:"); !ok {
		return protocol.DeprecatedDeviceConfig_SOC_UNSPECIFIED, errors.New("failed to find Intel family")
	} else if family != "6" {
		return protocol.DeprecatedDeviceConfig_SOC_UNSPECIFIED, errors.Errorf("unknown Intel family: %s", family)
	}

	modelStr, ok := parsed.find("Model:")
	if !ok {
		return protocol.DeprecatedDeviceConfig_SOC_UNSPECIFIED, errors.New("failed to find Intel model")
	}
	model, err := strconv.ParseInt(modelStr, 10, 64)
	if err != nil {
		return protocol.DeprecatedDeviceConfig_SOC_UNSPECIFIED, errors.Wrapf(err, "failed to parse Intel model: %q", modelStr)
	}
	switch model {
	case INTEL_FAM6_KABYLAKE_L:
		// AMBERLAKE_Y, COMET_LAKE_U, WHISKEY_LAKE_U, KABYLAKE_U, KABYLAKE_U_R, and
		// KABYLAKE_Y share the same model. Parse model name.
		// Note that Pentium brand is unsupported.
		modelName, ok := parsed.find("Model name:")
		if !ok {
			return protocol.DeprecatedDeviceConfig_SOC_UNSPECIFIED, errors.New("failed to find Intel model name")
		}
		for _, e := range []struct {
			soc protocol.DeprecatedDeviceConfig_SOC
			ptn string
		}{
			// https://ark.intel.com/content/www/us/en/ark/products/codename/186968/amber-lake-y.html
			{protocol.DeprecatedDeviceConfig_SOC_AMBERLAKE_Y, `Core.* [mi]\d-(10|8)\d{3}Y`},

			// https://ark.intel.com/content/www/us/en/ark/products/codename/90354/comet-lake.html
			{protocol.DeprecatedDeviceConfig_SOC_COMET_LAKE_U, `Core.* i\d-10\d{3}U|Celeron.* 5[23]05U|Intel\(R\) Pentium\(R\) CPU 6405U`},

			// https://ark.intel.com/content/www/us/en/ark/products/codename/135883/whiskey-lake.html
			{protocol.DeprecatedDeviceConfig_SOC_WHISKEY_LAKE_U, `Core.* i\d-8\d{2}5U|Celeron.* 4[23]05U`},

			// https://ark.intel.com/content/www/us/en/ark/products/codename/82879/kaby-lake.html
			{protocol.DeprecatedDeviceConfig_SOC_KABYLAKE_U, `Core.* i\d-7\d{3}U|Celeron.* 3[89]65U`},
			{protocol.DeprecatedDeviceConfig_SOC_KABYLAKE_Y, `Core.* [mi]\d-7Y\d{2}|Celeron.* 3965Y`},

			// https://ark.intel.com/content/www/us/en/ark/products/codename/126287/kaby-lake-r.html
			{protocol.DeprecatedDeviceConfig_SOC_KABYLAKE_U_R, `Core.* i\d-8\d{2}0U|Celeron.* 3867U`},
		} {
			r := regexp.MustCompile(e.ptn)
			if r.MatchString(modelName) {
				return e.soc, nil
			}
		}
		return protocol.DeprecatedDeviceConfig_SOC_UNSPECIFIED, errors.Errorf("unknown model name: %s", modelName)
	case INTEL_FAM6_ICELAKE_L:
		return protocol.DeprecatedDeviceConfig_SOC_ICE_LAKE_Y, nil
	case INTEL_FAM6_ATOM_GOLDMONT_PLUS:
		return protocol.DeprecatedDeviceConfig_SOC_GEMINI_LAKE, nil
	case INTEL_FAM6_ATOM_TREMONT_L:
		return protocol.DeprecatedDeviceConfig_SOC_JASPER_LAKE, nil
	case INTEL_FAM6_TIGERLAKE_L:
		return protocol.DeprecatedDeviceConfig_SOC_TIGER_LAKE, nil
	case INTEL_FAM6_ALDERLAKE_L:
		return protocol.DeprecatedDeviceConfig_SOC_ALDER_LAKE, nil
	case INTEL_FAM6_METEORLAKE_L:
		return protocol.DeprecatedDeviceConfig_SOC_METEOR_LAKE, nil
	case INTEL_FAM6_CANNONLAKE_L:
		return protocol.DeprecatedDeviceConfig_SOC_CANNON_LAKE_Y, nil
	case INTEL_FAM6_ATOM_GOLDMONT:
		return protocol.DeprecatedDeviceConfig_SOC_APOLLO_LAKE, nil
	case INTEL_FAM6_SKYLAKE_L:
		// SKYLAKE_U and SKYLAKE_Y share the same model. Parse model name.
		modelName, ok := parsed.find("Model name:")
		if !ok {
			return protocol.DeprecatedDeviceConfig_SOC_UNSPECIFIED, errors.New("failed to find Intel model name")
		}
		for _, e := range []struct {
			soc protocol.DeprecatedDeviceConfig_SOC
			ptn string
		}{
			// https://ark.intel.com/content/www/us/en/ark/products/codename/37572/skylake.html
			{protocol.DeprecatedDeviceConfig_SOC_SKYLAKE_U, `Core.* i\d-6\d{3}U|Celeron.*3[89]55U`},
			{protocol.DeprecatedDeviceConfig_SOC_SKYLAKE_Y, `Core.* m\d-6Y\d{2}`},
		} {
			r := regexp.MustCompile(e.ptn)
			if r.MatchString(modelName) {
				return e.soc, nil
			}
		}
		return protocol.DeprecatedDeviceConfig_SOC_UNSPECIFIED, errors.Errorf("unknown model name: %s", modelName)
	case INTEL_FAM6_ATOM_AIRMONT:
		return protocol.DeprecatedDeviceConfig_SOC_BRASWELL, nil
	case INTEL_FAM6_BROADWELL:
		return protocol.DeprecatedDeviceConfig_SOC_BROADWELL, nil
	case INTEL_FAM6_HASWELL, INTEL_FAM6_HASWELL_L:
		return protocol.DeprecatedDeviceConfig_SOC_HASWELL, nil
	case INTEL_FAM6_IVYBRIDGE:
		return protocol.DeprecatedDeviceConfig_SOC_IVY_BRIDGE, nil
	case INTEL_FAM6_ATOM_SILVERMONT:
		return protocol.DeprecatedDeviceConfig_SOC_BAY_TRAIL, nil
	case INTEL_FAM6_SANDYBRIDGE:
		return protocol.DeprecatedDeviceConfig_SOC_SANDY_BRIDGE, nil
	case INTEL_FAM6_ATOM_BONNELL:
		return protocol.DeprecatedDeviceConfig_SOC_PINE_TRAIL, nil
	default:
		return protocol.DeprecatedDeviceConfig_SOC_UNSPECIFIED, errors.Errorf("unknown Intel model: %d", model)
	}
}

// findAMDSOC returns an AMD SOC configuration based on "Model" field.
func findAMDSOC(parsed *lscpuResult) (protocol.DeprecatedDeviceConfig_SOC, error) {
	model, ok := parsed.find("Model:")
	if !ok {
		return protocol.DeprecatedDeviceConfig_SOC_UNSPECIFIED, errors.New("failed to find AMD model")
	}
	if model == "112" {
		return protocol.DeprecatedDeviceConfig_SOC_STONEY_RIDGE, nil
	}
	if family, ok := parsed.find("CPU family:"); ok {
		if family == "23" {
			if model == "24" || model == "32" {
				return protocol.DeprecatedDeviceConfig_SOC_PICASSO, nil
			} else if model == "160" {
				return protocol.DeprecatedDeviceConfig_SOC_MENDOCINO, nil
			}
		} else if family == "25" {
			if model == "80" {
				return protocol.DeprecatedDeviceConfig_SOC_CEZANNE, nil
			} else if model == "116" {
				return protocol.DeprecatedDeviceConfig_SOC_PHOENIX, nil
			}
		}
	}

	return protocol.DeprecatedDeviceConfig_SOC_UNSPECIFIED, errors.Errorf("unknown AMD model: %s", model)
}

// lsblk command output differs depending on the version. Attempt parsing in multiple ways to accept all the cases.

// lsblk from util-linux 2.32
type blockDevices2_32 struct {
	Name      string `json:"name"`
	Removable string `json:"rm"`
	Size      string `json:"size"`
	Type      string `json:"type"`
}

type lsblkRoot2_32 struct {
	BlockDevices []blockDevices2_32 `json:"blockdevices"`
}

// lsblk from util-linux 2.36.1
type blockDevices struct {
	Name      string `json:"name"`
	Removable bool   `json:"rm"`
	Size      int64  `json:"size"`
	Type      string `json:"type"`
}

type lsblkRoot struct {
	BlockDevices []blockDevices `json:"blockdevices"`
}

func parseLsblk2_32(jsonData []byte) (*lsblkRoot, error) {
	var old lsblkRoot2_32
	err := json.Unmarshal(jsonData, &old)
	if err != nil {
		return nil, err
	}

	var r lsblkRoot
	for _, e := range old.BlockDevices {
		s, err := strconv.ParseInt(e.Size, 10, 64)
		if err != nil {
			return nil, err
		}
		var rm bool
		if e.Removable == "0" || e.Removable == "" {
			rm = false
		} else if e.Removable == "1" {
			rm = true
		} else {
			return nil, fmt.Errorf("unknown value for rm: %q", e.Removable)
		}
		r.BlockDevices = append(r.BlockDevices, blockDevices{
			Name:      e.Name,
			Removable: rm,
			Size:      s,
			Type:      e.Type,
		})
	}
	return &r, nil
}

func parseLsblk2_36(jsonData []byte) (*lsblkRoot, error) {
	var r lsblkRoot
	err := json.Unmarshal(jsonData, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func parseLsblk(jsonData []byte) (*lsblkRoot, error) {
	var errs []error
	parsers := []func([]byte) (*lsblkRoot, error){parseLsblk2_36, parseLsblk2_32}
	for _, p := range parsers {
		r, err := p(jsonData)
		if err == nil {
			return r, nil
		}
		errs = append(errs, err)
	}
	var errStrings []string
	for _, e := range errs {
		errStrings = append(errStrings, e.Error())
	}
	return nil, fmt.Errorf("failed to parse JSON in all the expected formats: %s", strings.Join(errStrings, "; "))
}

// findDiskSize detects the size of the storage device from "lsblk -J -b" output in bytes.
// When there are multiple disks, returns the size of the largest one.
func findDiskSize(jsonData []byte) (int64, error) {
	r, err := parseLsblk(jsonData)
	if err != nil {
		return 0, err
	}
	var maxSize int64
	var found bool
	for _, x := range r.BlockDevices {
		if x.Type == "disk" && !x.Removable && !strings.HasPrefix(x.Name, "zram") {
			found = true
			if x.Size > maxSize {
				maxSize = x.Size
			}
		}
	}
	if !found {
		return 0, errors.New("no disk device found")
	}
	return maxSize, nil
}

// findMemorySize parses a content of /proc/meminfo and returns the total memory size in bytes.
func findMemorySize(meminfo []byte) (int64, error) {
	r := bytes.NewReader(meminfo)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		tokens := strings.SplitN(line, ":", 2)
		if len(tokens) != 2 || strings.TrimSpace(tokens[0]) != "MemTotal" {
			continue
		}
		cap := strings.SplitN(strings.TrimSpace(tokens[1]), " ", 2)
		if len(cap) != 2 {
			return 0, fmt.Errorf("unexpected line format: input=%s", line)
		}
		if cap[1] != "kB" {
			return 0, fmt.Errorf("unexpected unit: got %s, want kB; input=%s", cap[1], line)
		}
		val, err := strconv.ParseInt(cap[0], 10, 64)
		if err != nil {
			return 0, err
		}
		// meminfo reported kB is 2^10 bytes not 10^3.
		return val << 10, nil
	}
	return 0, fmt.Errorf("MemTotal not found; input=%q", string(meminfo))
}

func matchCrasDeviceType(pattern string) (*configpb.HardwareFeatures_Count, error) {
	b, err := exec.Command("cras_test_client").Output()
	if err != nil {
		return nil, err
	}
	if regexp.MustCompile(pattern).Match(b) {
		return &configpb.HardwareFeatures_Count{Value: 1}, nil
	}
	return &configpb.HardwareFeatures_Count{Value: 0}, nil
}

// findSpeakerAmplifier parses a content of in "/sys/kernel/debug/asoc/components"
// and returns the speaker amplifier used.
func findSpeakerAmplifier() (*configpb.Component_Amplifier, error) {

	// This sys path exists only on kernel >=4.14. But we don't
	// target amp tests on earlier kernels.
	f, err := os.Open("/sys/kernel/debug/asoc/components")
	if err != nil {
		return &configpb.Component_Amplifier{}, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if amp, found := matchSpeakerAmplifier(scanner.Text()); found {
			if enabled, err := bootTimeCalibration(); err == nil && enabled {
				amp.Features = append(amp.Features, configpb.Component_Amplifier_BOOT_TIME_CALIBRATION)
			}
			return amp, err
		}
	}
	return &configpb.Component_Amplifier{}, nil
}

var ampsRegexp = map[string]*regexp.Regexp{
	configpb.HardwareFeatures_Audio_MAX98357.String(): regexp.MustCompile(`^(i2c-)?ma?x98357a?((:\d*)|([_-]?\d*))?$`),
	configpb.HardwareFeatures_Audio_MAX98373.String(): regexp.MustCompile(`^(i2c-)?ma?x98373((:\d*)|([_-]?\d*))?$`),
	configpb.HardwareFeatures_Audio_MAX98360.String(): regexp.MustCompile(`^(i2c-)?ma?x98360a?((:\d*)|([_-]?\d*))?$`),
	configpb.HardwareFeatures_Audio_RT1015.String():   regexp.MustCompile(`^(i2c-)?((rtl?)|(10ec))?1015(\.\d*)?((:\d*)|([_-]?\d*))?$`),
	configpb.HardwareFeatures_Audio_RT1015P.String():  regexp.MustCompile(`^(i2c-)?(rtl?)?(10ec)?1015p(\.\d*)?((:\d*)|([_-]?\d*))?$`),
	configpb.HardwareFeatures_Audio_ALC1011.String():  regexp.MustCompile(`^(i2c-)?((rtl?)|(10ec))?1011(\.\d*)?((:\d*)|([_-]?\d*))?$`),
	configpb.HardwareFeatures_Audio_MAX98390.String(): regexp.MustCompile(`^(i2c-)?ma?x98390((:\d*)|(\.\d-\d+)|([_-]?\d*))?$`),
	configpb.HardwareFeatures_Audio_CS35L41.String():  regexp.MustCompile(`^(i2c-)?csc3541((:\d*)|([_-]?\d*))?$`),
}

func matchSpeakerAmplifier(line string) (*configpb.Component_Amplifier, bool) {
	for amp, re := range ampsRegexp {
		if re.MatchString(strings.ToLower(line)) {
			return &configpb.Component_Amplifier{Name: amp}, true
		}
	}
	return nil, false
}

// bootTimeCalibration returns whether the boot time calibration is
// enabled by parsing the sound_card_init config.
func bootTimeCalibration() (bool, error) {
	config, err := crosConfig("/audio/main", "sound-card-init-conf")
	if err != nil {
		return false, err
	}
	path := "/etc/sound_card_init/" + config
	if _, err := os.Stat(path); err != nil {
		// Regard config non-existence as boot_time_calibration disabled.
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return false, errors.New("failed to read sound_card_init config")
	}
	return isBootTimeCalibrationEnabled(string(b))
}

func isBootTimeCalibrationEnabled(s string) (bool, error) {
	re := regexp.MustCompile(`boot_time_calibration_enabled\s*?:\s*?(true|false)`)
	match := re.FindStringSubmatch(s)
	if match == nil {
		return false, errors.New("invalid sound_card_init config")
	}
	enabled := match[1]
	return enabled == "true", nil
}

func wifiFeatures() (*configpb.HardwareFeatures_Wifi, error) {
	dev, err := wlan.DeviceInfo()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get device")
	}

	_, err = exec.Command("vpd", "-g", "wifi_sar").Output()
	vpdSarFound := err == nil

	return &configpb.HardwareFeatures_Wifi{
		WifiChips: []configpb.HardwareFeatures_Wifi_WifiChip{
			configpb.HardwareFeatures_Wifi_WifiChip(dev.ID)},
		WifiVpdSar: vpdSarFound,
	}, nil
}

// hasBuiltinAudio tells if a given form factor has built-in audio devices
func hasBuiltinAudio(ctx context.Context, ff configpb.HardwareFeatures_FormFactor_FormFactorType) bool {
	switch ff {
	case configpb.HardwareFeatures_FormFactor_CLAMSHELL,
		configpb.HardwareFeatures_FormFactor_CONVERTIBLE,
		configpb.HardwareFeatures_FormFactor_DETACHABLE,
		configpb.HardwareFeatures_FormFactor_CHROMEBASE,
		configpb.HardwareFeatures_FormFactor_CHROMESLATE:
		return true
	case configpb.HardwareFeatures_FormFactor_CHROMEBIT,
		configpb.HardwareFeatures_FormFactor_CHROMEBOX:
		return false
	default:
		logging.Infof(ctx, "Unknown form factor: %s", ff)
		return false
	}
}

// cameraFeatures returns the list of configured camera features for the given
// |model| by inspecting the on-device feature config file.
func cameraFeatures(model string) ([]string, error) {
	type modelConfig map[string]struct {
		FeatureSet []map[string]interface{} `json:"feature_set"`
	}
	const featureProfilePath = "/etc/camera/feature_profile.json"
	jsonInput, err := ioutil.ReadFile(featureProfilePath)
	if err != nil {
		return nil, errors.Wrap(err, "cannot load feature profile config")
	}
	conf := make(modelConfig)
	if err := json.Unmarshal(jsonInput, &conf); err != nil {
		return nil, errors.Wrap(err, "cannot unmarshal feature profile config")
	}
	c, ok := conf[model]
	if !ok {
		return nil, errors.Errorf("feature set config for model %s doesn't exist", model)
	}
	featureSet := make(map[string]bool)
	for _, f := range c.FeatureSet {
		var v interface{}
		// The "type" attribute is always a string.
		if v, ok = f["type"]; !ok {
			continue
		}
		// There can be multiple entries for a feature with different
		// constraints.
		if _, ok := featureSet[v.(string)]; !ok {
			featureSet[v.(string)] = true
		}
	}
	var ret []string
	for k := range featureSet {
		ret = append(ret, k)
	}
	return ret, nil
}

// cameraUsbEnumerated returns the list of enumerated USB cameras.
func cameraUsbEnumerated() ([]string, error) {
	var ids []string
	out, err := exec.Command("media_v4l2_test", "--list_builtin_usbcam").Output()
	if err != nil {
		return nil, errors.Wrap(err, "failed to run media_v4l2_test")
	}
	for _, name := range strings.Fields(string(out)) {
		path := "/sys/class/video4linux/" + filepath.Base(name) + "/device"
		path, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, err
		}
		path = filepath.Dir(string(path))

		out, err := os.ReadFile(path + "/idVendor")
		if err != nil {
			return nil, err
		}
		vid := strings.TrimSuffix(string(out), "\n")

		out, err = os.ReadFile(path + "/idProduct")
		if err != nil {
			return nil, err
		}
		pid := strings.TrimSuffix(string(out), "\n")
		ids = append(ids, vid+":"+pid)
	}
	return ids, nil
}

// cameraEnumerated returns whether camera devices have been all enumerated, together with
// the list of enumerated USB cameras.
func cameraEnumerated() (bool, []string, error) {
	countStr, err := crosConfig("/camera", "count")
	if err != nil {
		return false, nil, errors.Wrap(err, "failed to get count from crosConfig")
	}
	count, err := strconv.Atoi(countStr)
	if err != nil {
		return false, nil, errors.Wrap(err, "failed to parse crosConfig output")
	}

	usbCamIds, err := cameraUsbEnumerated()
	if err != nil {
		return false, nil, errors.Wrap(err, "failed to get enumerated USB cams")
	}
	usbCams := len(usbCamIds)
	mipiCams := func() int {
		out, err := exec.Command("cros-camera-tool", "modules", "list").Output()
		if err != nil {
			return 0
		}
		var cams []map[string]string
		if err := json.Unmarshal(out, &cams); err != nil {
			return 0
		}
		return len(cams)
	}()

	return usbCams+mipiCams == count, usbCamIds, nil
}

// findGSCKeyID parses a content of "gsctool -a -f -M" and return a required key
func findGSCKeyID(str, keyIDType string) (string, error) {
	re := regexp.MustCompile(`(?m)^keyids: RO (0x.+), RW (0x.+)$`)

	switch keyIDType {
	case "RO":
		keyID := re.FindAllStringSubmatch(str, -1)[0][1]
		return keyID, nil
	case "RW":
		keyID := re.FindAllStringSubmatch(str, -1)[0][2]
		return keyID, nil
	default:
		return "", errors.Errorf("Unknown keyId type %s", keyIDType)
	}
}

// containsGSCKeyID returns true if reqKeyID is in the keyIDs
func containsGSCKeyID(keyIDs []GSCKeyID, reqKeyID GSCKeyID) bool {
	for _, keyID := range keyIDs {
		if keyID == reqKeyID {
			return true
		}
	}
	return false
}

// For mocking.
var flashromExtractCoreBootCmd = func(ctx context.Context, corebootBinName string) error {
	return exec.CommandContext(ctx, "flashrom", "-p", "internal", "-r", "-i", fmt.Sprintf("FW_MAIN_A:%s", corebootBinName)).Run()
}
var cbfsToolExtractCmd = func(ctx context.Context, corebootBinName, fileName, outputPath string) error {
	return exec.CommandContext(ctx, "cbfstool", corebootBinName, "extract", "-n", fileName, "-f", outputPath).Run()
}

var configLineRegexp = regexp.MustCompile(`^(# )?(CONFIG\S*)(=(y)| (is not set))`)

// parseKConfigs updates the provided HardwareFeatures with the features found
// by reading reading through the BIOS Kconfigs.
func parseKConfigs(ctx context.Context, features *configpb.HardwareFeatures) error {
	corebootBin, err := ioutil.TempFile("/var/tmp", "")
	if err != nil {
		return errors.Wrap(err, "failed to create temp file")
	}
	corebootBin.Close()
	defer os.Remove(corebootBin.Name())

	fwConfig, err := ioutil.TempFile("/var/tmp", "")
	if err != nil {
		return errors.Wrap(err, "failed to create temp file")
	}
	fwConfig.Close()
	defer os.Remove(fwConfig.Name())

	if err := flashromExtractCoreBootCmd(ctx, corebootBin.Name()); err != nil {
		return errors.Wrap(err, "failed to extract FW_MAIN_A bios section")
	}
	if err := cbfsToolExtractCmd(ctx, corebootBin.Name(), "config", fwConfig.Name()); err != nil {
		return errors.Wrap(err, "failed to extract bios Kconfig file")
	}
	inFile, err := os.Open(fwConfig.Name())
	if err != nil {
		return errors.Wrap(err, "failed to read bios Kconfig file")
	}
	defer inFile.Close()

	importantConfigs := map[string]*configpb.HardwareFeatures_Present{
		"CONFIG_MAINBOARD_HAS_EARLY_LIBGFXINIT": &features.FwConfig.MainboardHasEarlyLibgfxinit,
		"CONFIG_VBOOT_CBFS_INTEGRATION":         &features.FwConfig.VbootCbfsIntegration,
		"CONFIG_BMP_LOGO":                       &features.FwConfig.BmpLogo,
		"CONFIG_CHROMEOS_FW_SPLASH_SCREEN":      &features.FwConfig.FwSplashScreen,
	}

	scanner := bufio.NewScanner(inFile)
	for scanner.Scan() {
		line := scanner.Text()
		if match := configLineRegexp.FindStringSubmatch(line); match != nil {
			if val, ok := importantConfigs[match[2]]; ok {
				if match[4] == "y" {
					*val = configpb.HardwareFeatures_PRESENT
				} else if match[5] == "is not set" {
					*val = configpb.HardwareFeatures_NOT_PRESENT
				}
			}
		}
	}
	return nil
}

func parseECBuildConfig(ctx context.Context, features *configpb.HardwareFeatures) error {
	corebootBin, err := ioutil.TempFile("/var/tmp", "")
	if err != nil {
		return errors.Wrap(err, "failed to create temp file")
	}
	corebootBin.Close()
	defer os.Remove(corebootBin.Name())

	ecConfig, err := ioutil.TempFile("/var/tmp", "")
	if err != nil {
		return errors.Wrap(err, "failed to create temp file")
	}
	ecConfig.Close()
	defer os.Remove(ecConfig.Name())

	if err := flashromExtractCoreBootCmd(ctx, corebootBin.Name()); err != nil {
		return errors.Wrap(err, "failed to extract FW_MAIN_A bios section")
	}
	if err := cbfsToolExtractCmd(ctx, corebootBin.Name(), "ecrw.config", ecConfig.Name()); err != nil {
		return errors.Wrap(err, "failed to extract ec config file")
	}
	inFile, err := os.Open(ecConfig.Name())
	if err != nil {
		return errors.Wrap(err, "failed to read ec Kconfig file")
	}
	defer inFile.Close()

	ecBuildConfig := make(map[string]configpb.HardwareFeatures_Present)
	scanner := bufio.NewScanner(inFile)
	for scanner.Scan() {
		line := scanner.Text()
		if match := configLineRegexp.FindStringSubmatch(line); match != nil {
			if match[4] == "y" {
				ecBuildConfig[match[2]] = configpb.HardwareFeatures_PRESENT
			} else if match[5] == "is not set" {
				ecBuildConfig[match[2]] = configpb.HardwareFeatures_NOT_PRESENT
			} else {
				return errors.Errorf("Malformed config line: %s", line)
			}
		}
	}
	features.EmbeddedController.BuildConfig = ecBuildConfig
	return nil
}

// hasRuntimeProbeConfig returns true if the corresponding probe config for the
// model of the DUT exists.  The err is set if the error os.Stat returns is not
// fs.ErrNotExist.
func hasRuntimeProbeConfig(model string, encrypted bool) (bool, error) {
	probeConfigRelPath := "etc/runtime_probe/" + model + "/probe_config.json"
	if encrypted {
		probeConfigRelPath += ".enc"
	}
	configRoots := []string{
		"/usr/local/",
		"/",
	}
	for _, configRoot := range configRoots {
		_, err := os.Stat(configRoot + probeConfigRelPath)
		if err == nil {
			return true, nil
		}
		if !os.IsNotExist(err) {
			return false, err
		}
	}
	return false, nil
}

// featureLevel returns the feature level number of the DUT.
func featureLevel() (level uint32, err error) {
	cmd := exec.Command("feature_explorer", "--feature_level")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, errors.Wrapf(err, "failed to call feature_explorer %s", string(out))
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(out)), 0, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse feature level %s", string(out))
	}
	return uint32(value), nil
}

func oemName() string {
	if out, err := crosConfig("/branding", "oem-name"); err == nil {
		if out != "" {
			return out
		}
	}

	if out, err := os.ReadFile("/sys/firmware/vpd/ro/oem_name"); err == nil {
		if string(out) != "" {
			return string(out)
		}
	}

	return ""
}
