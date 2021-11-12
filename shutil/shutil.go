// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package shutil provides shell-related utility functions.
package shutil

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// The character class \w is equivalent to [0-9A-Za-z_]. Leading equals sign is unsafe in zsh,
	// see http://zsh.sourceforge.net/Doc/Release/Expansion.html#g_t_0060_003d_0027-expansion.
	leadingSafeChars  = `-\w@%+:,./`
	trailingSafeChars = leadingSafeChars + "="
)

// safeRE matches an argument that can be literally included in a shell
// command line without requiring escaping.
var safeRE = regexp.MustCompile(fmt.Sprintf("^[%s][%s]*$", leadingSafeChars, trailingSafeChars))

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
