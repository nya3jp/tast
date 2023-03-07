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
	"strconv"
	"strings"
	"time"

	configpb "go.chromium.org/chromiumos/config/go/api"

	"chromiumos/tast/errors"
	"chromiumos/tast/framework/protocol"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/testing/wlan"
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
		return "", errors.Errorf("cros_config failed (stderr: %q): %v", buf.Bytes(), err)
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

	config := &protocol.DeprecatedDeviceConfig{
		Id: &protocol.DeprecatedConfigId{
			Platform: platform,
			Model:    model,
			Brand:    brand,
		},
		Soc:             info.soc,
		Cpu:             info.cpuArch,
		HasNvmeSelfTest: false,
		HasVboot2:       vboot2,
	}
	features := &configpb.HardwareFeatures{
		Screen:                &configpb.HardwareFeatures_Screen{},
		Fingerprint:           &configpb.HardwareFeatures_Fingerprint{},
		EmbeddedController:    &configpb.HardwareFeatures_EmbeddedController{},
		Storage:               &configpb.HardwareFeatures_Storage{},
		Memory:                &configpb.HardwareFeatures_Memory{},
		Audio:                 &configpb.HardwareFeatures_Audio{},
		PrivacyScreen:         &configpb.HardwareFeatures_PrivacyScreen{},
		Soc:                   &configpb.HardwareFeatures_Soc{},
		Touchpad:              &configpb.HardwareFeatures_Touchpad{},
		Keyboard:              &configpb.HardwareFeatures_Keyboard{},
		FormFactor:            &configpb.HardwareFeatures_FormFactor{},
		DpConverter:           &configpb.HardwareFeatures_DisplayPortConverter{},
		Wifi:                  &configpb.HardwareFeatures_Wifi{},
		Cellular:              &configpb.HardwareFeatures_Cellular{},
		Bluetooth:             &configpb.HardwareFeatures_Bluetooth{},
		Hps:                   &configpb.HardwareFeatures_Hps{},
		Battery:               &configpb.HardwareFeatures_Battery{},
		Camera:                &configpb.HardwareFeatures_Camera{},
		TrustedPlatformModule: &configpb.HardwareFeatures_TrustedPlatformModule{},
		FwConfig:              &configpb.HardwareFeatures_FirmwareConfiguration{},
		RuntimeProbeConfig:    &configpb.HardwareFeatures_RuntimeProbeConfig{},
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
	case configpb.HardwareFeatures_FormFactor_CLAMSHELL, configpb.HardwareFeatures_FormFactor_CONVERTIBLE:
		features.Keyboard.KeyboardType = configpb.HardwareFeatures_Keyboard_INTERNAL
	case configpb.HardwareFeatures_FormFactor_DETACHABLE:
		features.Keyboard.KeyboardType = configpb.HardwareFeatures_Keyboard_DETACHABLE
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

	hasKeyboardBacklight, err := func() (string, error) {
		out, err := crosConfig("/power", "has-keyboard-backlight")
		if err != nil {
			return "", err
		}
		return out, nil
	}()
	if err != nil {
		logging.Infof(ctx, "Unknown /power/has-keyboard-backlight: %v", err)
	}

	hasKeyboardBacklightUnderPowerManager, err := func() (bool, error) {
		const fileName = "/usr/share/power_manager/has_keyboard_backlight"
		content, err := ioutil.ReadFile(fileName)
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, errors.Errorf("failed to read file %q: %v", fileName, err)
		}
		return strings.TrimSuffix(string(content), "\n") == "1", nil
	}()
	if err != nil {
		logging.Infof(ctx, "Unknown /usr/share/power_manager: %v", err)
	}

	switch hasKeyboardBacklight {
	case "1":
		features.Keyboard.Backlight = configpb.HardwareFeatures_PRESENT
	case "":
		if keyboardBacklight || hasKeyboardBacklightUnderPowerManager {
			features.Keyboard.Backlight = configpb.HardwareFeatures_PRESENT
		}
	default:
		features.Keyboard.Backlight = configpb.HardwareFeatures_NOT_PRESENT
	}

	hasInternalDisplay := func() bool {
		const drmSysFS = "/sys/class/drm"

		drmFiles, err := ioutil.ReadDir(drmSysFS)
		if err != nil {
			return false
		}

		// eDP displays show up as card*-eDP-1
		// MIPI panels show up as card*-DSI-1
		// Virtual displays in VMs show up as card*-Virtual-1
		cardMatch := regexp.MustCompile(`^card[0-9]-(eDP|DSI|Virtual)-1$`)
		for _, file := range drmFiles {
			fileName := file.Name()

			if cardMatch.MatchString(fileName) {
				if cardConnected, err := ioutil.ReadFile(path.Join(drmSysFS, fileName, "status")); err != nil {
					if !os.IsNotExist(err) {
						return false
					}
				} else {
					return strings.HasPrefix(string(cardConnected), "connected")
				}
			}
		}

		// No indication of internal panel connected and recognised.
		return false
	}()
	if hasInternalDisplay {
		features.Screen.PanelProperties = &configpb.Component_DisplayPanel_Properties{}
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
	features.Fingerprint.Location = configpb.HardwareFeatures_Fingerprint_NOT_PRESENT
	if hasFingerprint {
		features.Fingerprint.Location = configpb.HardwareFeatures_Fingerprint_LOCATION_UNKNOWN
	}

	// Device has ChromeEC if /dev/cros_ec exists.
	// TODO(b/173741162): Pull EmbeddedController data directly from Boxster.
	if _, err := os.Stat("/dev/cros_ec"); err == nil {
		features.EmbeddedController.Present = configpb.HardwareFeatures_PRESENT
		features.EmbeddedController.EcType = configpb.HardwareFeatures_EmbeddedController_EC_CHROME
		// Check for EC_FEATURE_TYPEC_CMD.
		output, err := exec.Command("ectool", "inventory").Output()
		if err != nil {
			features.EmbeddedController.FeatureTypecCmd = configpb.HardwareFeatures_PRESENT_UNKNOWN
		} else {
			// The presence of the integer "41" in the inventory output is a sufficient check, since 41 is
			// the bit position associated with this feature.
			if bytes.Contains(output, []byte("41")) {
				features.EmbeddedController.FeatureTypecCmd = configpb.HardwareFeatures_PRESENT
			} else {
				features.EmbeddedController.FeatureTypecCmd = configpb.HardwareFeatures_NOT_PRESENT
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

	if err := exec.Command("cros_config", "/modem", "firmware-variant").Run(); err != nil {
		logging.Infof(ctx, "Modem not found: %v", err)
		features.Cellular.Present = configpb.HardwareFeatures_NOT_PRESENT
	} else {
		features.Cellular.Present = configpb.HardwareFeatures_PRESENT
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

	hasEmmcStorage := func() bool {
		matches, err := filepath.Glob("/dev/mmc*")
		if err != nil {
			return false
		}
		if len(matches) > 0 {
			return true
		}
		return false
	}()
	if hasEmmcStorage {
		features.Storage.StorageType = configpb.Component_Storage_EMMC
	}

	// TODO(b/173741162): Pull storage information from boxster config and add
	// additional storage types.
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
	if hasNvmeStorage {
		features.Storage.StorageType = configpb.Component_Storage_NVME
	}

	// TODO(b/211755998): Pull information from boxster config after this got supported in boxster.
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
		SizeMegabytes: int32(memoryBytes / 1_000_000),
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
		// Check if privacy-screen prop is present.
		result := strings.Contains(string(value), "privacy-screen:")

		return result
	}()
	if hasPrivacyScreen {
		features.PrivacyScreen.Present = configpb.HardwareFeatures_PRESENT
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

	if err := parseKConfigs(ctx, features); err != nil {
		logging.Info(ctx, "Failed to parse BIOS kConfig: ", err)
	}

	rpConfigPresent, err := hasRuntimeProbeConfig(model)
	if err != nil {
		logging.Info(ctx, "Failed to determine if the config of Runtime Probe exists: ", err)
	}
	if rpConfigPresent {
		features.RuntimeProbeConfig.Present = configpb.HardwareFeatures_PRESENT
	} else {
		logging.Infof(ctx, "Config of Runtime Probe not found")
		features.RuntimeProbeConfig.Present = configpb.HardwareFeatures_NOT_PRESENT
	}

	return &protocol.HardwareFeatures{
		HardwareFeatures:       features,
		DeprecatedDeviceConfig: config,
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
		if family == "23" && (model == "24" || model == "32") {
			return protocol.DeprecatedDeviceConfig_SOC_PICASSO, nil
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
		return val * 1_000, nil
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
	return exec.CommandContext(ctx, "flashrom", "-p", "host", "-r", "-i", fmt.Sprintf("FW_MAIN_A:%s", corebootBinName)).Run()
}
var cbfsToolExtractConfigCmd = func(ctx context.Context, corebootBinName, fwConfigName string) error {
	return exec.CommandContext(ctx, "cbfstool", corebootBinName, "extract", "-n", "config", "-f", fwConfigName).Run()
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
	if err := cbfsToolExtractConfigCmd(ctx, corebootBin.Name(), fwConfig.Name()); err != nil {
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

// hasRuntimeProbeConfig returns true if the corresponding probe config for the
// model of the DUT exists.  The err is set if the error os.Stat returns is not
// fs.ErrNotExist.
func hasRuntimeProbeConfig(model string) (bool, error) {
	probeConfigRelPath := "etc/runtime_probe/" + model + "/probe_config.json"
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
