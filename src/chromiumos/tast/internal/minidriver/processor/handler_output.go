// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"chromiumos/tast/internal/logging"
)

// copyOutputHandler copies test output files to the host system when a test
// finishes.
type copyOutputHandler struct {
	baseHandler
	pull    PullFunc
	pullers sync.WaitGroup
}

var _ Handler = &copyOutputHandler{}

// NewCopyOutputHandler creates a handler which copies test output files to
// the host system when a test finishes.
func NewCopyOutputHandler(pull PullFunc) *copyOutputHandler {
	return &copyOutputHandler{
		pull: pull,
	}
}

// EntityCopyEnd handles EntityCopyEnd event.
// We cannot use EntityEnd here, because EntityEnd doesn't guarantee files are
// already copied, and Tast CLI may see incomplete output files before external
// bundle finishes coping the file.
func (h *copyOutputHandler) EntityCopyEnd(ctx context.Context, ei *entityInfo) error {
	// IntermediateOutDir can be empty for skipped tests.
	if ei.IntermediateOutDir == "" {
		return nil
	}

	// Pull finished test output files in a separate goroutine.
	h.pullers.Add(1)
	go func() {
		defer h.pullers.Done()
		if err := moveTestOutputData(h.pull, ei.IntermediateOutDir, ei.FinalOutDir); err != nil {
			// This may be written to a log of an irrelevant test.
			logging.Infof(ctx, "Failed to copy output data of %s: %v", ei.Entity.GetName(), err)
		}
	}()
	return nil
}

func (h *copyOutputHandler) RunEnd(ctx context.Context) {
	// Wait for output file pullers to finish.
	h.pullers.Wait()
}

// moveTestOutputData moves per-test output data using pull. dstDir is the path
// to the destination directory, typically ending with testName. dstDir should
// already exist.
func moveTestOutputData(pull PullFunc, outDir, dstDir string) error {
	const testOutputFileRenameExt = ".from_test"

	tmpDir, err := ioutil.TempDir(filepath.Dir(dstDir), "pulltmp.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	srcDir := filepath.Join(tmpDir, "files")
	if err := pull(outDir, srcDir); err != nil {
		return err
	}

	files, err := ioutil.ReadDir(srcDir)
	if err != nil {
		return err
	}
	for _, fi := range files {
		src := filepath.Join(srcDir, fi.Name())
		dst := filepath.Join(dstDir, fi.Name())

		// Check that the destination file doesn't already exist.
		// This could happen if a test creates an output file named log.txt.
		if _, err := os.Stat(dst); err == nil {
			dst += testOutputFileRenameExt
		}

		if err := os.Rename(src, dst); err != nil {
			return err
		}
	}
	return nil
}
