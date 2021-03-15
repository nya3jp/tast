// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"path/filepath"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/internal/linuxssh"
	"chromiumos/tast/ssh"
)

// pushToHost is a wrapper around linuxssh.PutFiles that should be used instead of calling PutFiles directly.
// dstDir is appended to cfg.HstCopyBasePath to support unit tests.
// Symbolic links are dereferenced to support symlinked data files: https://crbug.com/927424
func pushToHost(ctx context.Context, cfg *config.Config, hst *ssh.Conn, files map[string]string) (bytes int64, err error) {
	if cfg.HstCopyBasePath != "" {
		rewritten := make(map[string]string)
		for src, dst := range files {
			rewritten[src] = filepath.Join(cfg.HstCopyBasePath, dst)
		}
		files = rewritten
	}

	return linuxssh.PutFiles(ctx, hst, files, linuxssh.DereferenceSymlinks)
}

// deleteFromHost is a wrapper around hst.DeleteTree that should be used instead of calling DeleteTree directly.
// baseDir is appended to cfg.HstCopyBasePath to support unit tests.
func deleteFromHost(ctx context.Context, cfg *config.Config, hst *ssh.Conn, baseDir string, files []string) error {
	return linuxssh.DeleteTree(ctx, hst, filepath.Join(cfg.HstCopyBasePath, baseDir), files)
}
