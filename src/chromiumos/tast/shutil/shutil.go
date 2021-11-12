// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package shutil provides shell-related utility functions.
package shutil

import (
	"go.chromium.org/tast/shutil"
)

// Escape escapes a string so it can be safely included as an argument in a shell command line.
// The string is not modified if it can already be safely included.
func Escape(s string) string {
	return shutil.Escape(s)
}

// EscapeSlice escapes a slice of strings so each will be treated as a separate
// argument in the returned shell command line. See Escape for more information.
func EscapeSlice(args []string) string {
	return shutil.EscapeSlice(args)
}
