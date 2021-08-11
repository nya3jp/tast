// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package autocaps determines the DUT's capabilities by parsing autotest-capability YAML files.
//
// autotest-capability is a pre-Tast system developed to permit Autotest-based video tests to
// examine the DUT's capabilities to e.g. determine which codecs are supported.
//
// Capabilities are mostly statically assigned in the system image via layered YAML files in
// /usr/local/etc/autotest-capability, but in order to support the same system image being
// shared across different SKUs, "detectors" can also be used to determine capabilities at
// runtime based on the CPU model and the presence of a Kepler PCI device.
//
// See https://chromium.googlesource.com/chromiumos/overlays/chromiumos-overlay/+/HEAD/chromeos-base/autotest-capability-default/
// for more information about autotest-capability.
package autocaps

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v2"
)

const (
	// DefaultCapabilityDir is the directory where yaml files are installed by autotest-capability.
	DefaultCapabilityDir = "/usr/local/etc/autotest-capability"

	// ManagedFile is a path to the file listing all know capabilities.
	ManagedFile = "managed-capabilities.yaml"
)

// State describes the state of a capability.
type State int

const (
	// Yes indicates that the capability is satisfied.
	Yes State = iota
	// No indicates that the capability is unsatisfied.
	No
	// Disable is functionally equivalent to No. It is set to temporarily disable the capability.
	Disable
)

func (s State) String() string {
	switch s {
	case Yes:
		return "Yes"
	case No:
		return "No"
	default:
		return "Disable"
	}
}

var fileRegexp *regexp.Regexp      // matches YAML base filenames containing capabilities
var directiveRegexp *regexp.Regexp // matches directive line in capability files

func init() {
	// These come from client/cros/video/device_capability.py in the Autotest repo.
	fileRegexp = regexp.MustCompile(`^[0-9]+-.*\.yaml$`)
	directiveRegexp = regexp.MustCompile(`(?:(disable|no)\s+)?([\w\-]+)$`)
}

// Read reads YAML files specifying capabilities from the directory at dir.
// info is used to determine which capabilities are set; if nil, system info will be loaded by this function.
// A map containing all managed capabilities with their corresponding states is returned.
//
// Tests should not call this function or check capabilities directly. Instead, they should declare dependencies on
// required capabilities as described at https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/test_dependencies.md.
func Read(dir string, info *SysInfo) (map[string]State, error) {
	if info == nil {
		var err error
		if info, err = loadSysInfo(); err != nil {
			return nil, fmt.Errorf("failed loading system info: %v", err)
		}
	}

	var managed []string
	if err := decodeYAMLFile(filepath.Join(dir, ManagedFile), &managed); err != nil {
		return nil, err
	}

	caps := make(map[string]State, len(managed))
	for _, c := range managed {
		caps[c] = No
	}

	// ioutil.ReadDir returns sorted filenames.
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, fi := range fis {
		if !fileRegexp.MatchString(fi.Name()) {
			continue
		}
		path := filepath.Join(dir, fi.Name())
		if err := readCapsFile(path, info, caps); err != nil {
			return nil, fmt.Errorf("failed to read %v: %v", path, err)
		}
	}

	return caps, nil
}

// readCapsFile reads the YAML file at path and updates the set of capabilities in caps.
// An error is returned if any capabilities not already present in caps are seen.
func readCapsFile(path string, info *SysInfo, caps map[string]State) error {
	var items []capItem
	if err := decodeYAMLFile(path, &items); err != nil {
		return err
	}

	for _, item := range items {
		var directives []string
		var err error
		if item.directive != "" {
			directives = []string{item.directive}
		} else if directives, err = runDetector(&item.detectRule, info); err != nil {
			return err
		}
		for _, d := range directives {
			if err := applyDirective(d, caps); err != nil {
				return err
			}
		}
	}

	return nil
}

// applyDirective applies the supplied directive to the capabilities in caps.
//
// A directive can consist of:
//	- a bare capability name to set the capability's state to Yes
//	- "no <capability>" to set the capability's state to No
//	- "disable <capability>" to set the capability's state to Disable
func applyDirective(directive string, caps map[string]State) error {
	matches := directiveRegexp.FindStringSubmatch(directive)
	if matches == nil {
		return fmt.Errorf("invalid directive %q", directive)
	}

	cp := matches[2]
	if _, ok := caps[cp]; !ok {
		return fmt.Errorf("unknown capability %q", cp)
	}

	if s := matches[1]; s == "no" {
		caps[cp] = No
	} else if s == "disable" {
		caps[cp] = Disable
	} else {
		caps[cp] = Yes
	}
	return nil
}

// decodeYAMLFile reads a YAML file and decodes its contents to out.
func decodeYAMLFile(path string, out interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return yaml.NewDecoder(f).Decode(out)
}

// detectRule represents a rule in a capabilities file describing how a detector should
// be run to modify capabilities.
type detectRule struct {
	Detector     string   `yaml:"detector"`
	Match        []string `yaml:"match"`
	Capabilities []string `yaml:"capabilities"`
}

// capItem contains the contents of an item in a capabilities file. Each item can contain
// either a directive (see applyDirective) or a detectRule.
type capItem struct {
	directive string
	detectRule
}

// UnmarshalYAML implements yaml.Unmarshaler. A custom implementation must be specified
// to support unmarshaling into either the directive string or the detectRule struct.
func (ci *capItem) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if err := unmarshal(&ci.directive); err == nil {
		return nil
	}
	return unmarshal(&ci.detectRule)
}
