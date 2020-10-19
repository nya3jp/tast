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
	"strconv"
	"strings"

	configpb "go.chromium.org/chromiumos/config/go/api"
	"go.chromium.org/chromiumos/infra/proto/go/device"

	"chromiumos/tast/autocaps"
	"chromiumos/tast/errors"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/dep"
	"chromiumos/tast/internal/expr"
	"chromiumos/tast/lsbrelease"
)

const autotestCapPrefix = "autotest-capability:" // prefix for autotest-capability feature names

// handleGetDUTInfo handles a GetDUTInfoMode request from args
// and JSON-marshals a GetDUTInfoResult struct to w.
func handleGetDUTInfo(args *Args, cfg *Config, w io.Writer) error {
	features, warnings, err := getSoftwareFeatures(
		cfg.SoftwareFeatureDefinitions, cfg.USEFlagsFile, cfg.LSBReleaseFile, args.GetDUTInfo.ExtraUSEFlags, cfg.AutotestCapabilityDir)
	if err != nil {
		return err
	}

	var dc *device.Config
	var hwFeatures *configpb.HardwareFeatures
	if args.GetDUTInfo.RequestDeviceConfig {
		var ws []string
		dc, hwFeatures, ws = newDeviceConfigAndHardwareFeatures()
		warnings = append(warnings, ws...)
	}

	osVersion := cfg.OSVersion

	res := GetDUTInfoResult{
		SoftwareFeatures: features,
		DeviceConfig:     dc,
		HardwareFeatures: hwFeatures,
		OSVersion:        osVersion,
		Warnings:         warnings,
	}
	if err := json.NewEncoder(w).Encode(&res); err != nil {
		return command.NewStatusErrorf(statusError, "failed to serialize into JSON: %v", err)
	}
	return nil
}

// getSoftwareFeatures implements the main function of GetDUTInfoMode (i.e., except input/output
// conversion for RPC).
func getSoftwareFeatures(definitions map[string]string, useFlagsFile, lsbReleaseFile string, extraUSEFlags []string, autotestCapsDir string) (
	features *dep.SoftwareFeatures, warnings []string, err error) {
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

	if lsbReleaseFile == "" {
		warnings = append(warnings, "lsb-release path is not specified; board names in software feature definitions will not work")
	} else if lr, err := lsbrelease.LoadFrom(lsbReleaseFile); err != nil {
		warnings = append(warnings, fmt.Sprintf("failed to read lsbrelease; board names in software feature definitions will not work: %v", err))
	} else if board, ok := lr[lsbrelease.Board]; !ok {
		warnings = append(warnings, fmt.Sprintf("failed to find boardname in lsbrelease; board names in software feature definitions will not work"))
	} else {
		flags = append(flags, "board:"+board)
	}

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
	*dep.SoftwareFeatures, error) {
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
	return &dep.SoftwareFeatures{Available: available, Unavailable: unavailable}, nil
}

// newDeviceConfigAndHardwareFeatures returns a device.Config and api.HardwareFeatures instances
// some of whose members are filled based on runtime information.
func newDeviceConfigAndHardwareFeatures() (dc *device.Config, retFeatures *configpb.HardwareFeatures, warns []string) {
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

	soc, err := findSOC()
	if err != nil {
		warns = append(warns, fmt.Sprintf("Unknown SOC: %v", err))
	}
	config := &device.Config{
		Id: &device.ConfigId{
			PlatformId: platform,
			ModelId:    model,
			BrandId:    brand,
		},
		Soc: soc,
	}
	features := &configpb.HardwareFeatures{
		Screen:      &configpb.HardwareFeatures_Screen{},
		Fingerprint: &configpb.HardwareFeatures_Fingerprint{},
	}

	hasInternalDisplay := func() bool {
		const drmSysFS = "/sys/class/drm"

		drmFiles, err := ioutil.ReadDir(drmSysFS)
		if err != nil {
			return false
		}

		// eDP displays show up as card*-eDP-1
		// MIPI panels show up as card*-DSI-1
		cardMatch := regexp.MustCompile(`^card[0-9]-(eDP|DSI)-1$`)
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
		features.Screen.TouchSupport = configpb.HardwareFeatures_PRESENT

		// Kept for protocol compatibility with an older version of Tast command.
		// TODO(crbug.com/1094802): Remove this when we bump sourceCompatVersion in tast/internal/build/compat.go.
		config.HardwareFeatures = append(config.HardwareFeatures, device.Config_HARDWARE_FEATURE_TOUCHSCREEN)
	}

	hasFingerprint := func() bool {
		fi, err := os.Stat("/dev/cros_fp")
		if err != nil {
			return false
		}
		return (fi.Mode() & os.ModeCharDevice) != 0
	}()
	features.Fingerprint.Location = configpb.HardwareFeatures_Fingerprint_NOT_PRESENT
	if hasFingerprint {
		features.Fingerprint.Location = configpb.HardwareFeatures_Fingerprint_LOCATION_UNKNOWN
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

	return config, features, warns
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

func findSOC() (device.Config_SOC, error) {
	b, err := exec.Command("lscpu", "--json").Output()
	if err != nil {
		return device.Config_SOC_UNSPECIFIED, err
	}
	var parsed lscpuResult
	if err := json.Unmarshal(b, &parsed); err != nil {
		return device.Config_SOC_UNSPECIFIED, errors.Wrap(err, "failed to parse lscpu result")
	}
	vendorID, ok := parsed.find("Vendor ID:")
	if !ok {
		return device.Config_SOC_UNSPECIFIED, errors.New("vendor ID not found")
	}

	switch vendorID {
	case "ARM":
		return findArmSOC()
	case "Qualcomm":
		return findQualcommSOC(&parsed)
	case "GenuineIntel":
		return findIntelSOC(&parsed)
	case "AuthenticAMD":
		return findAMDSOC(&parsed)
	default:
		return device.Config_SOC_UNSPECIFIED, errors.Errorf("unknown vendor ID: %q", vendorID)
	}
}

func findArmSOC() (device.Config_SOC, error) {
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
			case "jep106:0426:8192":
				return device.Config_SOC_MT8192, nil
			default:
				return device.Config_SOC_UNSPECIFIED, errors.Errorf("unknown ARM model: %s", socID)
			}
		}
	}

	// For old platforms with SMCCC < 1.2: mt8173, mt8183, rk3288, rk3399,
	// match with their compatible string. Obtain the string after the last , and trim \x00.
	// Example: google,krane-sku176\x00google,krane\x00mediatek,mt8183\x00
	c, err := ioutil.ReadFile("/sys/firmware/devicetree/base/compatible")
	if err != nil {
		return device.Config_SOC_UNSPECIFIED, errors.Wrap(err, "ARM model not found")
	}

	compatible := string(c)
	model := strings.ToLower(compatible[strings.LastIndex(compatible, ",")+1:])
	model = strings.TrimRight(model, "\x00")

	switch model {
	case "mt8173":
		return device.Config_SOC_MT8173, nil
	case "mt8183":
		return device.Config_SOC_MT8183, nil
	case "rk3288":
		return device.Config_SOC_RK3288, nil
	case "rk3399":
		return device.Config_SOC_RK3399, nil
	default:
		return device.Config_SOC_UNSPECIFIED, errors.Errorf("unknown ARM model: %s", model)
	}
}

func findQualcommSOC(parsed *lscpuResult) (device.Config_SOC, error) {
	model, ok := parsed.find("Model:")
	if !ok {
		return device.Config_SOC_UNSPECIFIED, errors.New("Qualcomm model not found")
	}

	switch model {
	case "14":
		return device.Config_SOC_SC7180, nil
	default:
		return device.Config_SOC_UNSPECIFIED, errors.Errorf("unknown Qualcomm model: %s", model)
	}
}

func findIntelSOC(parsed *lscpuResult) (device.Config_SOC, error) {
	if family, ok := parsed.find("CPU family:"); !ok {
		return device.Config_SOC_UNSPECIFIED, errors.New("Intel family not found")
	} else if family != "6" {
		return device.Config_SOC_UNSPECIFIED, errors.Errorf("unknown Intel family: %s", family)
	}

	modelStr, ok := parsed.find("Model:")
	if !ok {
		return device.Config_SOC_UNSPECIFIED, errors.New("Intel model not found")
	}
	model, err := strconv.ParseInt(modelStr, 10, 64)
	if err != nil {
		return device.Config_SOC_UNSPECIFIED, errors.Wrapf(err, "failed to parse intel model: %q", modelStr)
	}
	switch model {
	case INTEL_FAM6_KABYLAKE_L:
		// AMBERLAKE_Y, COMET_LAKE_U, WHISKEY_LAKE_U, KABYLAKE_U, KABYLAKE_U_R, and
		// KABYLAKE_Y share the same model. Parse model name.
		// Note that Pentium brand is unsupported.
		modelName, ok := parsed.find("Model name:")
		if !ok {
			return device.Config_SOC_UNSPECIFIED, errors.New("Intel model name not found")
		}
		for _, e := range []struct {
			soc device.Config_SOC
			ptn string
		}{
			// https://ark.intel.com/content/www/us/en/ark/products/codename/186968/amber-lake-y.html
			{device.Config_SOC_AMBERLAKE_Y, `Core.* i\d-(10|8)\d{3}Y`},

			// https://ark.intel.com/content/www/us/en/ark/products/codename/90354/comet-lake.html
			{device.Config_SOC_COMET_LAKE_U, `Core.* i\d-10\d{3}U|Celeron.* 5[23]05U`},

			// https://ark.intel.com/content/www/us/en/ark/products/codename/135883/whiskey-lake.html
			{device.Config_SOC_WHISKEY_LAKE_U, `Core.* i\d-8\d{2}5U|Celeron.* 4[23]05U`},

			// https://ark.intel.com/content/www/us/en/ark/products/codename/82879/kaby-lake.html
			{device.Config_SOC_KABYLAKE_U, `Core.* i\d-7\d{3}U|Celeron.* 3[89]65U`},
			{device.Config_SOC_KABYLAKE_Y, `Core.* [mi]\d-7Y\d{2}|Celeron.* 3965Y`},

			// https://ark.intel.com/content/www/us/en/ark/products/codename/126287/kaby-lake-r.html
			{device.Config_SOC_KABYLAKE_U_R, `Core.* i\d-8\d{2}0U|Celeron.* 3867U`},
		} {
			r := regexp.MustCompile(e.ptn)
			if r.MatchString(modelName) {
				return e.soc, nil
			}
		}
		return device.Config_SOC_UNSPECIFIED, errors.Errorf("unknown model name: %s", modelName)
	case INTEL_FAM6_ICELAKE_L:
		return device.Config_SOC_ICE_LAKE_Y, nil
	case INTEL_FAM6_ATOM_GOLDMONT_PLUS:
		return device.Config_SOC_GEMINI_LAKE, nil
	case INTEL_FAM6_ATOM_TREMONT_L:
		return device.Config_SOC_JASPER_LAKE, nil
	case INTEL_FAM6_TIGERLAKE_L:
		return device.Config_SOC_TIGER_LAKE, nil
	case INTEL_FAM6_CANNONLAKE_L:
		return device.Config_SOC_CANNON_LAKE_Y, nil
	case INTEL_FAM6_ATOM_GOLDMONT:
		return device.Config_SOC_APOLLO_LAKE, nil
	case INTEL_FAM6_SKYLAKE_L:
		// SKYLAKE_U and SKYLAKE_Y share the same model. Parse model name.
		modelName, ok := parsed.find("Model name:")
		if !ok {
			return device.Config_SOC_UNSPECIFIED, errors.New("Intel model name not found")
		}
		for _, e := range []struct {
			soc device.Config_SOC
			ptn string
		}{
			// https://ark.intel.com/content/www/us/en/ark/products/codename/37572/skylake.html
			{device.Config_SOC_SKYLAKE_U, `Core.* i\d-6\d{3}U|Celeron.*3[89]55U`},
			{device.Config_SOC_SKYLAKE_Y, `Core.* m\d-6Y\d{2}`},
		} {
			r := regexp.MustCompile(e.ptn)
			if r.MatchString(modelName) {
				return e.soc, nil
			}
		}
		return device.Config_SOC_UNSPECIFIED, errors.Errorf("unknown model name: %s", modelName)
	case INTEL_FAM6_ATOM_AIRMONT:
		return device.Config_SOC_BRASWELL, nil
	case INTEL_FAM6_BROADWELL:
		return device.Config_SOC_BROADWELL, nil
	case INTEL_FAM6_HASWELL, INTEL_FAM6_HASWELL_L:
		return device.Config_SOC_HASWELL, nil
	case INTEL_FAM6_IVYBRIDGE:
		return device.Config_SOC_IVY_BRIDGE, nil
	case INTEL_FAM6_ATOM_SILVERMONT:
		return device.Config_SOC_BAY_TRAIL, nil
	case INTEL_FAM6_SANDYBRIDGE:
		return device.Config_SOC_SANDY_BRIDGE, nil
	case INTEL_FAM6_ATOM_BONNELL:
		return device.Config_SOC_PINE_TRAIL, nil
	default:
		return device.Config_SOC_UNSPECIFIED, errors.Errorf("unknown Intel model: %d", model)
	}
}

func findAMDSOC(parsed *lscpuResult) (device.Config_SOC, error) {
	model, ok := parsed.find("Model:")
	if !ok {
		return device.Config_SOC_UNSPECIFIED, errors.New("AMD model not found")
	}
	if model == "112" {
		return device.Config_SOC_STONEY_RIDGE, nil
	}
	return device.Config_SOC_UNSPECIFIED, errors.Errorf("unknown AMD model: %s", model)
}
