// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package linuxssh

import (
	"context"
	"strings"

	"go.chromium.org/tast/core/ssh"
)

// ReadBootID reads the current boot_id at hst.
func ReadBootID(ctx context.Context, hst *ssh.Conn) (string, error) {
	out, err := hst.CommandContext(ctx, "cat", "/proc/sys/kernel/random/boot_id").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
