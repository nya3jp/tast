// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"fmt"
	"path/filepath"

	"chromiumos/tast/host"
)

// pushToHost is a wrapper around hst.PutTree that should be used instead of calling PutTree directly.
// dstDir is appended to cfg.hstCopyBasePath to support unit tests.
func pushToHost(ctx context.Context, cfg *Config, hst *host.SSH, srcDir, dstDir string,
	files []string) (bytes int64, err error) {
	// Actually run these commands in unit tests; see hstCopyAnnounceCmd.
	oldAnnounce := hst.AnnounceCmd
	defer func() { hst.AnnounceCmd = oldAnnounce }()
	hst.AnnounceCmd = cfg.hstCopyAnnounceCmd

	return hst.PutTree(ctx, srcDir, filepath.Join(cfg.hstCopyBasePath, dstDir), files)
}

// moveFromHost copies the tree rooted at src on hst to dst on the local system and deletes src from hst.
// src is appended to cfg.hstCopyBasePath to support unit tests.
func moveFromHost(ctx context.Context, cfg *Config, hst *host.SSH, src, dst string) error {
	// Actually run these commands in unit tests; see hstCopyAnnounceCmd.
	oldAnnounce := hst.AnnounceCmd
	defer func() { hst.AnnounceCmd = oldAnnounce }()
	hst.AnnounceCmd = cfg.hstCopyAnnounceCmd

	src = filepath.Join(cfg.hstCopyBasePath, src)
	cfg.Logger.Debugf("Copying %s from host to %s", src, dst)
	if err := hst.GetFile(ctx, src, dst); err != nil {
		return err
	}
	cfg.Logger.Debugf("Cleaning %s on host", src)
	if out, err := hst.Run(ctx, fmt.Sprintf("rm -rf %s", host.QuoteShellArg(src))); err != nil {
		cfg.Logger.Logf("Failed cleaning %s: %v\n%s", src, err, out)
	}
	return nil
}
