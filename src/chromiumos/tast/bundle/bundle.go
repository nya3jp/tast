// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"fmt"
	"os"

	"chromiumos/tast/internal/bundle"
)

// Delegate injects functions as a part of test bundle framework implementation.
type Delegate = bundle.Delegate

// lockStdIO replaces os.Stdin, os.Stdout and os.Stderr with closed pipes and
// returns the original files. This function can be called at the beginning of
// test bundles to ensure that calling fmt.Print and its family does not corrupt
// the control channel.
func lockStdIO() (stdin, stdout, stderr *os.File) {
	r, w, err := os.Pipe()
	if err != nil {
		panic(fmt.Sprint("Failed to lock stdio: ", err))
	}
	r.Close()
	w.Close()

	stdin = os.Stdin
	stdout = os.Stdout
	stderr = os.Stderr
	os.Stdin = r
	os.Stdout = w
	os.Stderr = w
	return stdin, stdout, stderr
}
