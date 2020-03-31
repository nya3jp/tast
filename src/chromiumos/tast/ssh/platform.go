// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import (
	"fmt"

	"chromiumos/tast/shutil"
)

// Platform defines platform-specific behaviours for SSH connections.
type Platform struct {
	// BuildShellCommand builds the shell command required to execute the given command
	// in the given directory on the target platform. args[0] is the name of the command
	// to be executed. If dir is empty (""), use the default working directory.
	BuildShellCommand func(dir string, args []string) string
}

// shellCmd builds a shell command string to execute a process with exec.
func shellCmd(dir string, args []string) string {
	cmd := "exec " + shutil.EscapeSlice(args)
	if dir != "" {
		// Return 125 (chosen arbitrarily) if dir does not exist.
		// TODO(nya): Consider handling the directory error more gracefully.
		cmd = fmt.Sprintf("cd %s > /dev/null 2>&1 || exit 125; %s", shutil.Escape(dir), cmd)
	}
	return cmd
}

// DefaultPlatform represents a system with a generic POSIX shell.
var DefaultPlatform = &Platform{BuildShellCommand: shellCmd}
