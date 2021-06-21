// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/internal/linuxssh"
	"chromiumos/tast/ssh"
)

// moveFromHost copies the tree rooted at src on hst to dst on the local system and deletes src from hst.
// src is appended to cfg.HstCopyBasePath to support unit tests.
func moveFromHost(ctx context.Context, cfg *config.Config, hst *ssh.Conn, src, dst string) error {
	if err := linuxssh.GetFile(ctx, hst, src, dst, linuxssh.PreserveSymlinks); err != nil {
		return err
	}
	if out, err := hst.Command("rm", "-rf", "--", src).Output(ctx); err != nil {
		cfg.Logger.Logf("Failed cleaning %s: %v\n%s", src, err, out)
	}
	return nil
}
