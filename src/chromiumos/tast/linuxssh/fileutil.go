// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package linuxssh defines linux specific SSH utilities.
package linuxssh

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"chromiumos/tast/host"
)

// GetFile copies a file or directory from the host to the local machine.
// dst is the full destination name for the file or directory being copied, not
// a destination directory into which it will be copied. dst will be replaced
// if it already exists.
func GetFile(ctx context.Context, conn host.Conn, src, dst string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	// Create a temporary directory alongside the destination path.
	td, err := ioutil.TempDir(filepath.Dir(dst), filepath.Base(dst)+".")
	if err != nil {
		return fmt.Errorf("creating local temp dir failed: %v", err)
	}
	defer os.RemoveAll(td)

	sb := filepath.Base(src)
	rcmd := conn.Command("tar", "-c", "--gzip", "-C", filepath.Dir(src), sb)
	p, err := rcmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %v", err)
	}
	if err := rcmd.Start(ctx); err != nil {
		return fmt.Errorf("running remote tar failed: %v", err)
	}
	defer rcmd.Wait(ctx)
	defer rcmd.Abort()

	cmd := exec.CommandContext(ctx, "/bin/tar", "-x", "--gzip", "--no-same-owner", "-C", td)
	cmd.Stdin = p
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running local tar failed: %v", err)
	}

	if err := os.Rename(filepath.Join(td, sb), dst); err != nil {
		return fmt.Errorf("moving local file failed: %v", err)
	}
	return nil
}
