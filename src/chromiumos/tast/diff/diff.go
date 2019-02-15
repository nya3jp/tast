// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package diff computes differences between strings.
package diff

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"

	"chromiumos/tast/errors"
)

// writeTempFile creates a temp file containing the given s. Returns the name
// of the created temp file.
func writeTempFile(s string) (string, error) {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(s); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// Diff takes the diff between orig and expect. Returns empty string if equals,
// otherwise returns the diff in unified diff format.
// In tests, it is recommended to output the diff result into a file, instead
// of directly outputting to log message.
func Diff(orig, expect string) (string, error) {
	if orig == expect {
		return "", nil
	}

	f1, err := writeTempFile(orig)
	if err != nil {
		return "", err
	}
	defer os.Remove(f1)

	f2, err := writeTempFile(expect)
	if err != nil {
		return "", err
	}
	defer os.Remove(f2)

	// Ignore error. diff command returns error if difference is found.
	out, _ := exec.Command("diff", "-ua", f1, f2).CombinedOutput()

	// Strip leading two lines, which are temp file name.
	parts := bytes.SplitN(out, []byte("\n"), 3)
	if len(parts) < 3 {
		return "", errors.New("Unexpected diff output")
	}
	return string(parts[2]), nil
}
