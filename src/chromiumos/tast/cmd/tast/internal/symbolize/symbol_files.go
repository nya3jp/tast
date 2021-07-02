// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package symbolize

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"

	"chromiumos/tast/cmd/tast/internal/symbolize/breakpad"
	"chromiumos/tast/internal/logging"
)

// symbolFileInfo contains a module's path and corresponding Breakpad ID. See breakpad.ModuleInfo.
type symbolFileInfo struct{ path, id string }

// createSymbolFile attempts to create a symbol file within symDir for fi.
// Debug binaries should be located within buildRoot.
func createSymbolFile(fi *symbolFileInfo, symDir, buildRoot string) error {
	bin := breakpad.GetDebugBinaryPath(buildRoot, fi.path)
	if _, err := os.Stat(bin); os.IsNotExist(err) {
		return fmt.Errorf("no debug file %v", bin)
	}
	if mi, err := breakpad.WriteSymbolFile(bin, symDir); err != nil {
		return fmt.Errorf("failed to write symbol file: %v", err)
	} else if mi.ID != fi.id {
		return fmt.Errorf("wrote symbol file with ID %v (different build?)", mi.ID)
	}
	return nil
}

// createSymbolFiles attempts to create a symbol file within cfg.SymbolDir for each file listed in sf.
// Debug binaries should be located within cfg.BuildRoot.
// The number of symbol files that were successfully created is returned.
func createSymbolFiles(ctx context.Context, cfg *Config, sf breakpad.SymbolFileMap) (created int) {
	ch := make(chan *symbolFileInfo) // passes work to goroutines
	wg := sync.WaitGroup{}           // used to wait for goroutines
	var nc int32

	// Start a fixed number of worker goroutines so we don't launch a zillion dump_syms processes at once.
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			for fi := range ch {
				err := createSymbolFile(fi, cfg.SymbolDir, cfg.BuildRoot)
				if err != nil {
					logging.Debugf(ctx, "Failed to generate symbol file for %v with ID %v: %v", fi.path, fi.id, err)
				} else {
					logging.Debugf(ctx, "Generated symbol file for %v with ID %v", fi.path, fi.id)
					atomic.AddInt32(&nc, 1)
				}
			}
			wg.Done()
		}()
	}

	// Enqueue work and wait for all of the goroutines to complete.
	for path, id := range sf {
		ch <- &symbolFileInfo{path, id}
	}
	close(ch)
	wg.Wait()
	return int(nc)
}
