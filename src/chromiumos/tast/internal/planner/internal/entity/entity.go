// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package entity provides common operations for entities.
package entity

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"chromiumos/tast/internal/testing"
)

// State represents state associated with an entity.
type State interface {
	DataPath(p string) string
	Errorf(fmt string, args ...interface{})
}

// PreCheck checks the entity is able to run and otherwise reports errors.
func PreCheck(data []string, s State) {
	// Make sure all required data files exist.
	for _, fn := range data {
		fp := s.DataPath(fn)
		if _, err := os.Stat(fp); err == nil {
			continue
		}
		ep := fp + testing.ExternalErrorSuffix
		if data, err := ioutil.ReadFile(ep); err == nil {
			s.Errorf("Required data file %s missing: %s", fn, string(data))
		} else {
			s.Errorf("Required data file %s missing", fn)
		}
	}
}

// CreateOutDir creates an output directory for the entity with the given name.
func CreateOutDir(baseDir, name string) (string, error) {
	// baseDir can be blank for unit tests.
	if baseDir == "" {
		return "", nil
	}

	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", err
	}

	var outDir string
	// First try to make a fixed-name directory. This allows unit tests to be deterministic.
	if err := os.Mkdir(filepath.Join(baseDir, name), 0755); err == nil {
		outDir = filepath.Join(baseDir, name)
	} else if os.IsExist(err) {
		// The directory already exists. Use ioutil.TempDir to create a randomly named one.
		var err error
		outDir, err = ioutil.TempDir(baseDir, name+".")
		if err != nil {
			return "", err
		}
	} else {
		return "", err
	}

	// Make the directory world-writable so that tests can create files as other users,
	// and set the sticky bit to prevent users from deleting other users' files.
	// (The mode passed to os.MkdirAll is modified by umask, so we need an explicit chmod.)
	if err := os.Chmod(outDir, 0777|os.ModeSticky); err != nil {
		return "", err
	}
	return outDir, nil
}
