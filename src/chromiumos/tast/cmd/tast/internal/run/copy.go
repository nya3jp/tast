// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"path/filepath"

	"chromiumos/tast/host"
)

// pushToHost is a wrapper around hst.PutFiles that should be used instead of calling PutFiles directly.
// dstDir is appended to cfg.hstCopyBasePath to support unit tests.
// Symbolic links are dereferenced to support symlinked data files: https://crbug.com/927424
func pushToHost(ctx context.Context, cfg *Config, hst *host.SSH, files map[string]string) (bytes int64, err error) {
	undo := setAnnounceCmdForCopy(cfg, hst)
	defer undo()

	if cfg.hstCopyBasePath != "" {
		rewritten := make(map[string]string)
		for src, dst := range files {
			rewritten[src] = filepath.Join(cfg.hstCopyBasePath, dst)
		}
		files = rewritten
	}

	return hst.PutFiles(ctx, files, host.DereferenceSymlinks)
}

// moveFromHost copies the tree rooted at src on hst to dst on the local system and deletes src from hst.
// src is appended to cfg.hstCopyBasePath to support unit tests.
func moveFromHost(ctx context.Context, cfg *Config, hst *host.SSH, src, dst string) error {
	undo := setAnnounceCmdForCopy(cfg, hst)
	defer undo()

	src = filepath.Join(cfg.hstCopyBasePath, src)
	if err := hst.GetFile(ctx, src, dst); err != nil {
		return err
	}
	if out, err := hst.Command("rm", "-rf", "--", src).Output(ctx); err != nil {
		cfg.Logger.Logf("Failed cleaning %s: %v\n%s", src, err, out)
	}
	return nil
}

// deleteFromHost is a wrapper around hst.DeleteTree that should be used instead of calling DeleteTree directly.
// baseDir is appended to cfg.hstCopyBasePath to support unit tests.
func deleteFromHost(ctx context.Context, cfg *Config, hst *host.SSH, baseDir string, files []string) error {
	undo := setAnnounceCmdForCopy(cfg, hst)
	defer undo()

	return hst.DeleteTree(ctx, filepath.Join(cfg.hstCopyBasePath, baseDir), files)
}

// setAnnounceCmdForCopy is a helper function that configures hst to temporarily run the
// file-copy-related commands that are passed to it; it only has an effect in unit tests
// (where cfg.hstCopyAnnounceCmd may be non-nil). The returned function should be called
// when the file copy is completed to undo the change. See hstCopyAnnounceCmd for details.
func setAnnounceCmdForCopy(cfg *Config, hst *host.SSH) (undo func()) {
	old := hst.AnnounceCmd
	hst.AnnounceCmd = cfg.hstCopyAnnounceCmd
	return func() { hst.AnnounceCmd = old }
}
