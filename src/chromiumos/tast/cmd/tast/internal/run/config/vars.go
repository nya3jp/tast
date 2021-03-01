// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// findVarsFiles returns a list of paths to vars files under dir. The returned
// paths are sorted in a stable order. If dir doesn't exist, empty paths is
// returned with no error.
func findVarsFiles(dir string) (paths []string, err error) {
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if filepath.Ext(path) == ".yaml" {
			paths = append(paths, path)
		}
		return nil
	}); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("couldn't walk vars dir: %v", err)
	}
	return paths, nil
}

// readVarsFile reads a YAML file at path containing key-value pairs.
func readVarsFile(path string) (map[string]string, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	vars := make(map[string]string)
	if err := yaml.Unmarshal(b, &vars); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %v", path, err)
	}
	return vars, nil
}

// mergeVarsMode specifies the behavior of mergeVars when it finds duplicated
// entries.
type mergeVarsMode int

const (
	skipOnDuplicate  = iota // skip duplicated entries
	errorOnDuplicate        // error on duplicated entries
)

// mergeVars merges newVars into vars.
// Behavior on key duplication is specified by mode: if mode is skipOnDuplicate,
// an entry in newVars is skipped when vars already contains it; if mode is
// errorOnDuplicate, an error is returned.
// This function overwrites the given vars. vars must not be nil. In the case of
// errors, the value of vars is unspecified.
func mergeVars(vars, newVars map[string]string, mode mergeVarsMode) error {
	for k, v := range newVars {
		if _, ok := vars[k]; ok {
			if mode == skipOnDuplicate {
				continue
			}
			return fmt.Errorf("duplicated key %q", k)
		}
		vars[k] = v
	}
	return nil
}

// readAndMergeVarsFile reads a YAML file at path containing key-value pairs and
// merges it into vars. See readVarsFile and mergeVars.
func readAndMergeVarsFile(vars map[string]string, path string, mode mergeVarsMode) error {
	newVars, err := readVarsFile(path)
	if err != nil {
		return fmt.Errorf("failed to read vars from %s: %v", path, err)
	}
	if err := mergeVars(vars, newVars, mode); err != nil {
		return fmt.Errorf("failed to merge vars from %s: %v", path, err)
	}
	return nil
}
