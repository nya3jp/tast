// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testutil provides support code for unit tests.
package testutil

import (
	"testing"

	"go.chromium.org/tast/testutil"
)

// TempDir creates a temporary directory prefixed by "tast_unittest_[TestName]." and returns its path.
// If the directory cannot be created, a fatal error is reported to t.
func TempDir(t *testing.T) string {
	return testutil.TempDir(t)
}

// WriteFiles creates and writes files (keys are relative filenames,
// values are contents) within dir.
func WriteFiles(dir string, files map[string]string) error {
	return testutil.WriteFiles(dir, files)
}

// ReadFiles reads all regular files under dir and returns their
// relative paths and contents.
func ReadFiles(dir string) (map[string]string, error) {
	return testutil.ReadFiles(dir)
}

// AppendToFile appends data to the file at path.
func AppendToFile(path, data string) error {
	return testutil.AppendToFile(path, data)
}
