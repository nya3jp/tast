// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strings"

	"go.chromium.org/chromiumos/infra/proto/go/device"

	"chromiumos/tast/autocaps"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/expr"
)

const autotestCapPrefix = "autotest-capability:" // prefix for autotest-capability feature names

// handleGetDUTInfo handles a GetDUTInfoMode request from args
// and JSON-marshals a GetDUTInfoResult struct to w.
func handleGetDUTInfo(args *Args, cfg *Config, w io.Writer) error {
	features, warnings, err := getSoftwareFeatures(
		cfg.SoftwareFeatureDefinitions, cfg.USEFlagsFile, args.GetDUTInfo.ExtraUSEFlags, cfg.AutotestCapabilityDir)
	if err != nil {
		return err
	}

	var dc *device.Config
	if args.GetDUTInfo.RequestDeviceConfig {
		var ws []string
		dc, ws = newDeviceConfig()
		warnings = append(warnings, ws...)
	}

	res := GetDUTInfoResult{
		SoftwareFeatures: features,
		DeviceConfig:     dc,
		Warnings:         warnings,
	}
	if err := json.NewEncoder(w).Encode(&res); err != nil {
		return command.NewStatusErrorf(statusError, "failed to serialize into JSON: %v", err)
	}
	return nil
}

// getSoftwareFeatures implements the main function of GetDUTInfoMode (i.e., except input/output
// conversion for RPC).
func getSoftwareFeatures(definitions map[string]string, useFlagsFile string, extraUSEFlags []string, autotestCapsDir string) (
	features *SoftwareFeatures, warnings []string, err error) {
	if useFlagsFile == "" {
		return nil, nil, command.NewStatusErrorf(statusBadArgs, "feature enumeration unsupported")
	}

	// If the file listing USE flags doesn't exist, we're probably running on a non-test
	// image. Return an empty response to signal that to the caller.
	if _, err := os.Stat(useFlagsFile); os.IsNotExist(err) {
		return nil, nil, nil
	}

	flags, err := readUSEFlagsFile(useFlagsFile)
	if err != nil {
		return nil, nil, command.NewStatusErrorf(statusError, "failed to read %v: %v", useFlagsFile, err)
	}
	flags = append(flags, extraUSEFlags...)

	var autotestCaps map[string]autocaps.State
	if autotestCapsDir != "" {
		if ac, err := autocaps.Read(autotestCapsDir, nil); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", autotestCapsDir, err))
		} else {
			autotestCaps = ac
		}
	}

	features, err = determineSoftwareFeatures(definitions, flags, autotestCaps)
	if err != nil {
		return nil, nil, command.NewStatusErrorf(statusError, "%v", err)
	}
	return features, warnings, nil
}

// readUSEFlagsFile reads a list of USE flags from fn (see Config.USEFlagsFile).
// Each flag should be specified on its own line, and lines beginning with '#' are ignored.
func readUSEFlagsFile(fn string) ([]string, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var flags []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		flag := strings.TrimSpace(sc.Text())
		if flag != "" && flag[0] != '#' {
			flags = append(flags, flag)
		}
	}
	if err = sc.Err(); err != nil {
		return nil, err
	}
	return flags, nil
}

// determineSoftwareFeatures computes the DUT's available and unavailable software features.
// definitions maps feature names to definitions (see Config.SoftwareFeatureDefinitions).
// useFlags contains a list of relevant USE flags that were set when building the system image (see Config.USEFlagsFile).
// autotestCaps contains a mapping from autotest-capability names to the corresponding states.
func determineSoftwareFeatures(definitions map[string]string, useFlags []string, autotestCaps map[string]autocaps.State) (
	*SoftwareFeatures, error) {
	var available, unavailable []string
	for ft, es := range definitions {
		if strings.HasPrefix(ft, autotestCapPrefix) {
			return nil, fmt.Errorf("feature %q has reserved prefix %q", ft, autotestCapPrefix)
		}

		ex, err := expr.New(es)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %q feature expression %q: %v", ft, es, err)
		}
		if ex.Matches(useFlags) {
			available = append(available, ft)
		} else {
			unavailable = append(unavailable, ft)
		}
	}

	for name, state := range autotestCaps {
		if state == autocaps.Yes {
			available = append(available, autotestCapPrefix+name)
		} else {
			unavailable = append(unavailable, autotestCapPrefix+name)
		}
	}

	sort.Strings(available)
	sort.Strings(unavailable)
	return &SoftwareFeatures{Available: available, Unavailable: unavailable}, nil
}

// newDeviceConfig returns a device.Config instance some of whose members are filled
// based on runtime information.
func newDeviceConfig() (dc *device.Config, warns []string) {
	crosConfig := func(path, prop string) (string, error) {
		cmd := exec.Command("cros_config", path, prop)
		var buf bytes.Buffer
		cmd.Stderr = &buf
		b, err := cmd.Output()
		if err != nil {
			return "", errors.Errorf("cros_config failed (stderr: %q): %v", buf.Bytes(), err)
		}
		return string(b), nil
	}

	platform, err := func() (*device.PlatformId, error) {
		out, err := crosConfig("/identity", "platform-name")
		if err != nil {
			return nil, err
		}
		return &device.PlatformId{Value: out}, nil
	}()
	if err != nil {
		warns = append(warns, fmt.Sprintf("unknown platform-id: %v", err))
	}
	model, err := func() (*device.ModelId, error) {
		out, err := crosConfig("/", "name")
		if err != nil {
			return nil, err
		}
		return &device.ModelId{Value: out}, nil
	}()
	if err != nil {
		warns = append(warns, fmt.Sprintf("unknown model-id: %v", err))
	}
	brand, err := func() (*device.BrandId, error) {
		out, err := crosConfig("/", "brand-code")
		if err != nil {
			return nil, err
		}
		return &device.BrandId{Value: out}, nil
	}()
	if err != nil {
		warns = append(warns, fmt.Sprintf("unknown brand-id: %v", err))
	}
	config := &device.Config{
		Id: &device.ConfigId{
			PlatformId: platform,
			ModelId:    model,
			BrandId:    brand,
		},
	}

	hasInternalDisplay := func() bool {
		// Intel and AMD usually hang the panel at card0-eDP-1 (this
		// file only exists on those platforms).
		if card0Edid, err := ioutil.ReadFile("/sys/class/drm/card0-eDP-1/edid"); err != nil {
			if !os.IsNotExist(err) {
				return false
			}
		} else {
			return len(card0Edid) > 0
		}
		// ARM-based chromebooks hang the panel at card1-DSI-1.
		if card1Connected, err := ioutil.ReadFile("/sys/class/drm/card1-DSI-1/status"); err != nil {
			if !os.IsNotExist(err) {
				return false
			}
		} else {
			return strings.HasPrefix(string(card1Connected), "connected")
		}
		// No indication of internal panel connected and recognised.
		return false
	}()
	if hasInternalDisplay {
		config.HardwareFeatures = append(config.HardwareFeatures, device.Config_HARDWARE_FEATURE_INTERNAL_DISPLAY)
	}

	hasTouchScreen := func() bool {
		b, err := exec.Command("udevadm", "info", "--export-db").Output()
		if err != nil {
			return false
		}
		return regexp.MustCompile(`(?m)^E: ID_INPUT_TOUCHSCREEN=1$`).Match(b)
	}()
	if hasTouchScreen {
		config.HardwareFeatures = append(config.HardwareFeatures, device.Config_HARDWARE_FEATURE_TOUCHSCREEN)
	}

	hasFingerprint := func() bool {
		fi, err := os.Stat("/dev/cros_fp")
		if err != nil {
			return false
		}
		return (fi.Mode() & os.ModeCharDevice) != 0
	}()
	if hasFingerprint {
		config.HardwareFeatures = append(config.HardwareFeatures, device.Config_HARDWARE_FEATURE_FINGERPRINT)
	}

	func() {
		// This function determines DUT's power supply type and stores it to config.Power.
		// If DUT has a battery, config.Power is Config_POWER_SUPPLY_BATTERY.
		// If DUT has AC power supplies only, config.Power is Config_POWER_SUPPLY_AC_ONLY.
		// Otherwise, Config_POWER_SUPPLY_UNSPECIFIED is populated.
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
		config.Power = device.Config_POWER_SUPPLY_UNSPECIFIED
		files, err := ioutil.ReadDir(sysFsPowerSupplyPath)
		if err != nil {
			warns = append(warns, fmt.Sprintf("failed to read %v: %v", sysFsPowerSupplyPath, err))
			return
		}
		for _, file := range files {
			devPath := path.Join(sysFsPowerSupplyPath, file.Name())
			supplyTypeBytes, err := ioutil.ReadFile(path.Join(devPath, "type"))
			supplyType := strings.TrimSuffix(string(supplyTypeBytes), "\n")
			if err != nil {
				warns = append(warns, fmt.Sprintf("failed to read supply type of %v: %v", devPath, err))
				continue
			}
			if strings.HasPrefix(supplyType, "Battery") {
				supplyScopeBytes, err := ioutil.ReadFile(path.Join(devPath, "scope"))
				supplyScope := strings.TrimSuffix(string(supplyScopeBytes), "\n")
				if err != nil && !os.IsNotExist(err) {
					// Ignore NotExist error since /sys/class/power_supply/*/scope may not exist
					warns = append(warns, fmt.Sprintf("failed to read supply type of %v: %v", devPath, err))
					continue
				}
				if strings.HasPrefix(string(supplyScope), "Device") {
					// Ignore batteries for peripheral devices.
					continue
				}
				config.Power = device.Config_POWER_SUPPLY_BATTERY
				// Found at least one battery so this device is powered by battery.
				break
			}
			if !isACPower[supplyType] {
				warns = append(warns, fmt.Sprintf("Unknown supply type %v for %v", supplyType, devPath))
				continue
			}
			config.Power = device.Config_POWER_SUPPLY_AC_ONLY
		}
	}()

	return config, warns
}
