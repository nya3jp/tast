// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package shutil provides shell-related utility functions.
package shutil

import (
	"regexp"
	"strings"
)

// safeRE matches an argument that can be literally included in a shell
// command line without requiring escaping.
var safeRE = regexp.MustCompile(`^[A-Za-z0-9@%_+=:,./-]+$`)

// Escape escapes a string so it can be safely included as an argument in a shell command line.
// The string is not modified if it can already be safely included.
func Escape(s string) string {
	if safeRE.MatchString(s) {
		return s
	}
	return "'" + strings.Replace(s, "'", `'"'"'`, -1) + "'"
}

// EscapeSlice escapes a slice of strings so each will be treated as a separate
// argument in the returned shell command line. See Escape for more information.
func EscapeSlice(args []string) string {
	escaped := make([]string, len(args))
	for i, arg := range args {
		escaped[i] = Escape(arg)
	}
	return strings.Join(escaped, " ")
}
