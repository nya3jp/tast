// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakecmd is used to install a fake version of a command for testing.
package fakecmd

import (
	"fmt"
	"os"
	"path/filepath"
)

// Install puts a fake version of a command in the PATH and returns a clean up function.
func Install(cmd string) (func(), error) {
	const path = "PATH"

	// Make the fake gsutil executable. This also ensures the file exists.
	err := os.Chmod(cmd, 0700)
	if err != nil {
		return nil, fmt.Errorf("cannot chmod %q: %v", cmd, err)
	}

	cmdDir, err := filepath.Abs(filepath.Dir(cmd))
	if err != nil {
		return nil, fmt.Errorf("cannot find absolute path to %q: %v", cmd, err)
	}

	origPath := os.Getenv(path)
	os.Setenv(path, cmdDir+":"+origPath)

	return func() {
		os.Setenv(path, origPath)
	}, nil
}
