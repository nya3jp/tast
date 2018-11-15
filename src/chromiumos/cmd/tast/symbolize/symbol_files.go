// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package symbolize

import (
	"fmt"
	"os"
	"runtime"

	"chromiumos/cmd/tast/symbolize/breakpad"
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
// Files that are still missing are returned.
func createSymbolFiles(cfg *Config, sf breakpad.SymbolFileMap) (missing breakpad.SymbolFileMap) {
	ch := make(chan *symbolFileInfo)  // passes work to goroutines
	rch := make(chan *symbolFileInfo) // passes back missing files or nil for success

	// Start a fixed number of worker goroutines so we don't launch a zillion dump_syms processes at once.
	for i := 0; i < runtime.NumCPU(); i++ {
		go func() {
			for fi := range ch {
				if err := createSymbolFile(fi, cfg.symbolDir, cfg.buildRoot); err != nil {
					cfg.logger.Debugf("Failed to generate symbol file for %v with ID %v: %v", fi.path, fi.id, err)
					rch <- fi
				} else {
					cfg.logger.Debugf("Generated symbol file for %v with ID %v", fi.path, fi.id)
					rch <- nil
				}
			}
		}()
	}

	// Enqueue work and wait for results.
	for path, id := range sf {
		ch <- &symbolFileInfo{path, id}
	}
	missing = make(breakpad.SymbolFileMap)
	for i := 0; i < len(sf); i++ {
		if fi := <-rch; fi != nil {
			missing[fi.path] = fi.id
		}
	}
	return missing
}
