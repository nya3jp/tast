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
	"os"
	"os/exec"
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

	return config, warns
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
		return device.Config_SOC_UNSPECIFIED, errors.Errorf("vendor ID not found")
	}

	switch vendorID {
	case "ARM":
		return findArmSOC(&parsed)
	case "GenuineIntel":
		return findIntelSOC(&parsed)
	case "AuthenticAMD":
		return findAMDSOC(&parsed)
	default:
		return device.Config_SOC_UNSPECIFIED, errors.Errorf("unknown vendor ID: %q", vendorID)
	}
}

func findArmSOC(parsed *lscpuResult) (device.Config_SOC, error) {
	model, ok := parsed.find("Model:")
	if !ok {
		return device.Config_SOC_UNSPECIFIED, errors.New("ARM model not found")
	}

	// SOC_MT8176 and SOC_MT8183 are unsupported, since there are no devices.
	switch model {
	case "1":
		return device.Config_SOC_RK3288, nil
	case "2":
		return device.Config_SOC_MT8173, nil
	case "3":
		return device.Config_SOC_TEGRA_K1, nil
	case "4":
		return device.Config_SOC_RK3399, nil
	default:
		return device.Config_SOC_UNSPECIFIED, errors.Errorf("unknown ARM model: %s", model)
	}
}

func findIntelSOC(parsed *lscpuResult) (device.Config_SOC, error) {
	if family, ok := parsed.find("CPU family:"); !ok {
		return device.Config_SOC_UNSPECIFIED, errors.New("Intel family not found")
	} else if family != "6" {
		return device.Config_SOC_UNSPECIFIED, errors.Errorf("unknown Intel family: %s", family)
	}

	model, ok := parsed.find("Model:")
	if !ok {
		return device.Config_SOC_UNSPECIFIED, errors.New("Intel model not found")
	}
	switch model {
	case "142":
		// AMBERLAKE_Y, COMET_LAKE_U, WHISKEY_LAKE_U, KABYLAKE_U, KABYLAKE_U_R, and
		// KABYLAKE_Y share the same model. Parse model name.
		modelName, ok := parsed.find("Model name:")
		if !ok {
			return device.Config_SOC_UNSPECIFIED, errors.New("Intel model name not found")
		}
		for _, e := range []struct {
			soc device.Config_SOC
			ptn string
		}{
			{device.Config_SOC_AMBERLAKE_Y, `i7-8500Y|i5-(8210|8200)Y|m3-8100Y`},
			{device.Config_SOC_COMET_LAKE_U, `i7-(10710|10510)U|i5-10210U|i3-10110U|Pentium.*6405U|Celeron.*5205U`},
			{device.Config_SOC_WHISKEY_LAKE_U, `i7-(8665|8565)U|i5-(8365|8265)U|i3-8145U|Pentium.*5405U|Celeron.*4205U`},
			{device.Config_SOC_KABYLAKE_U, `i7-7500U|i5-7200U|i3-7100U`},
			{device.Config_SOC_KABYLAKE_U_R, `i7-(8650|8550)U|i5-(8350|8250)U|i3-8130U`},
			{device.Config_SOC_KABYLAKE_Y, `i7-7Y75|i5-7Y54|m3-7Y30`},
		} {
			r := regexp.MustCompile(e.ptn)
			if r.MatchString(modelName) {
				return e.soc, nil
			}
		}
		return device.Config_SOC_UNSPECIFIED, errors.Errorf("Unknown model name: %s", modelName)
	case "122":
		return device.Config_SOC_GEMINI_LAKE, nil
	case "108", "106":
		// These values are for ICE_LAKE_DP and SP.
		return device.Config_SOC_ICE_LAKE_Y, nil
	case "102":
		// The value is for CANNON_LAKE_U.
		return device.Config_SOC_CANNON_LAKE_Y, nil
	case "92":
		return device.Config_SOC_APOLLO_LAKE, nil
	case "78":
		// SKYLAKE_U and SKYLAKE_Y share the same model. Parse model name.
		modelName, ok := parsed.find("Model name:")
		if !ok {
			return device.Config_SOC_UNSPECIFIED, errors.New("Intel model name not found")
		}
		for _, e := range []struct {
			soc device.Config_SOC
			ptn string
		}{
			{device.Config_SOC_SKYLAKE_U, `i[357]-\d+U`},
			{device.Config_SOC_SKYLAKE_Y, `m[357]-6Y\d+`},
		} {
			r := regexp.MustCompile(e.ptn)
			if r.MatchString(modelName) {
				return e.soc, nil
			}
		}
		return device.Config_SOC_UNSPECIFIED, errors.Errorf("Unknown model name: %s", modelName)
	case "72":
		return device.Config_SOC_BRASWELL, nil
	case "71", "61":
		return device.Config_SOC_BROADWELL, nil
	case "70", "69", "60":
		return device.Config_SOC_HASWELL, nil
	case "58":
		return device.Config_SOC_IVY_BRIDGE, nil
	case "55":
		return device.Config_SOC_BAY_TRAIL, nil
	case "42":
		return device.Config_SOC_SANDY_BRIDGE, nil
	case "28":
		return device.Config_SOC_PINE_TRAIL, nil
	default:
		return device.Config_SOC_UNSPECIFIED, errors.Errorf("unknown Intel model: %s", model)
	}
}

func findAMDSOC(parsed *lscpuResult) (device.Config_SOC, error) {
	model, ok := parsed.find("Model:")
	if !ok {
		return device.Config_SOC_UNSPECIFIED, errors.New("Intel model not found")
	}
	if model == "112" {
		return device.Config_SOC_STONEY_RIDGE, nil
	}
	return device.Config_SOC_UNSPECIFIED, errors.Errorf("unknown AMD model: %s", model)
}
