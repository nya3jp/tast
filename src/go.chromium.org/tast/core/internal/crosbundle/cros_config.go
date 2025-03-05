// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crosbundle

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"sync"

	"gopkg.in/yaml.v2"
)

// BuildTargets The build-targets field of the configuration's firmware
type BuildTargets struct {
	Coreboot string `yaml:"coreboot"`
	Ish      string `yaml:"ish"`
}

// Firmware The firmware field of the configuration
type Firmware struct {
	BuildTargets BuildTargets `yaml:"build-targets"`
	ImageName    string       `yaml:"image-name"`
}

// Identity The identity field of the configuration
type Identity struct {
	SkuID string `yaml:"sku-id"`
}

// ConfigItem A single configuration instance
type ConfigItem struct {
	Firmware *Firmware `yaml:"firmware"`
	Identity Identity  `yaml:"identity"`
}

// ChromeOSConfig The top level configs list
type ChromeOSConfig struct {
	Configs []ConfigItem `yaml:"configs"`
}

// RootConfig The root level node of the configuration yaml
type RootConfig struct {
	ChromeOS ChromeOSConfig `yaml:"chromeos"`
}

var (
	cachedConfig RootConfig
	configLoaded bool
	configMutex  sync.Mutex
)

// parseConfigYaml Parse the config.yaml file on the DUT and return a
// RootConfig struct on success. To extend the returned config, modify
// the structs above to include the YAML fields required.
func parseConfigYaml(ctx context.Context) (RootConfig, error) {
	configMutex.Lock()
	defer configMutex.Unlock()

	if configLoaded {
		return cachedConfig, nil
	}

	yamlPath := "/usr/share/chromeos-config/yaml/config.yaml"
	if _, err := os.Stat(yamlPath); err != nil {
		return RootConfig{}, fmt.Errorf("Failed to get file stat: %v", err)
	}

	fileContent, err := os.ReadFile(yamlPath)
	if err != nil {
		return RootConfig{}, fmt.Errorf("Failed to read file: %v", err)
	}

	var config RootConfig
	err = yaml.Unmarshal(fileContent, &config)
	if err != nil {
		return config, fmt.Errorf("Failed to unmarshal YAML file: %v", err)
	}

	cachedConfig = config
	configLoaded = true

	return config, nil
}

// crosConfigForCurrentDevice Get the current configuration for the DUT. This is
// done by parsing the config.yaml file and running crosid to get the
// configuration index.
func crosConfigForCurrentDevice(ctx context.Context) (ConfigItem, error) {
	config, err := parseConfigYaml(ctx)
	if err != nil {
		return ConfigItem{}, err
	}

	crosIDOutput, err := exec.Command("crosid").Output()
	if err != nil {
		return ConfigItem{}, fmt.Errorf("Failed to get crosid: %v", err)
	}

	crosIDOutputStr := string(crosIDOutput)

	configIndexRegex := regexp.MustCompile(`CONFIG_INDEX='([^']+)'`)

	// Extract values
	configIndexMatch := configIndexRegex.FindStringSubmatch(crosIDOutputStr)

	if configIndexMatch == nil || len(configIndexMatch) != 2 {
		return ConfigItem{}, fmt.Errorf("Failed to match crosid output: %v", crosIDOutputStr)
	}

	// configIndexMatch[1] is the first capture group in the regex, which
	// contains the config index.
	configIndex, err := strconv.Atoi(configIndexMatch[1])
	if err != nil {
		return ConfigItem{}, fmt.Errorf("Failed to convert config index to int: %s", configIndexMatch[1])
	}

	return config.ChromeOS.Configs[configIndex], nil
}
