// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package crash can be used by local tests to interact with on-device crash reports.
package crash

import (
	"os"
	"path/filepath"
	"strings"

	"chromiumos/tast/fsutil"
)

const (
	// DefaultCrashDir contains the directory where the kernel writes core and minidump files.
	DefaultCrashDir = "/var/spool/crash"
	// ChromeCrashDir contains the directory where Chrome writes minidump files.
	// Tests configure Chrome to write crashes to this location/ by setting the BREAKPAD_DUMP_LOCATION
	// environment variable. This overrides the default /home/chronos/user/crash location, which is in
	// the user's cryptohome and hence is only accessible while they are logged in.
	ChromeCrashDir = "/home/chronos/crash"

	// BIOSExt is the extension for bios crash files.
	BIOSExt = ".bios_log"
	// CoreExt is the extension for core files.
	CoreExt = ".core"
	// MinidumpExt is the extension for minidump crash files.
	MinidumpExt = ".dmp"
	// LogExt is the extension for log files containing additional information that are written by crash_reporter.
	LogExt = ".log"
	// InfoExt is the extention for info files.
	InfoExt = ".info"
	// ProclogExt is the extention for proclog files.
	ProclogExt = ".proclog"
	// KCrashExt is the extension for log files created by kernel warnings and crashes.
	KCrashExt = ".kcrash"
	// GPUStateExt is the extension for GPU state files written by crash_reporter.
	GPUStateExt = ".i915_error_state.log.xz"
	// MetadataExt is the extension for metadata files written by crash collectors and read by crash_sender.
	MetadataExt = ".meta"
	// CompressedTxtExt is an extension on the compressed log files written by crash_reporter.
	CompressedTxtExt = ".txt.gz"
	// CompressedLogExt is an extension on the compressed log files written by crash_reporter.
	CompressedLogExt = ".log.gz"

	lsbReleasePath = "/etc/lsb-release"
)

// DefaultDirs returns all standard directories to which crashes are written.
func DefaultDirs() []string {
	return []string{DefaultCrashDir, ChromeCrashDir}
}

// isCrashFile returns true if filename could be the name of a file generated by
// crashes or crash_reporter.
func isCrashFile(filename string) bool {
	knownExts := []string{
		BIOSExt,
		CoreExt,
		MinidumpExt,
		LogExt,
		ProclogExt,
		InfoExt,
		KCrashExt,
		GPUStateExt,
		MetadataExt,
		CompressedTxtExt,
		CompressedLogExt,
	}
	for _, ext := range knownExts {
		if strings.HasSuffix(filename, ext) {
			return true
		}
	}
	return false
}

// GetCrashes returns the paths of all files in dirs generated in response to crashes.
// Nonexistent directories are skipped.
func GetCrashes(dirs ...string) ([]string, error) {
	var crashFiles []string
	for _, dir := range dirs {
		df, err := os.Open(dir)
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		files, err := df.Readdirnames(-1)
		df.Close()
		if err != nil {
			return nil, err
		}

		for _, fn := range files {
			if isCrashFile(fn) {
				crashFiles = append(crashFiles, filepath.Join(dir, fn))
			}
		}
	}
	return crashFiles, nil
}

// CopyNewFiles copies paths that are present in newPaths but not in oldPaths into dstDir.
// If maxPerExec is positive, it limits the maximum number of files that will be copied
// for each base executable. The returned warnings map contains non-fatal errors keyed by
// crash file paths.
func CopyNewFiles(dstDir string, newPaths, oldPaths []string) (warnings map[string]error, err error) {
	oldMap := make(map[string]struct{}, len(oldPaths))
	for _, p := range oldPaths {
		oldMap[p] = struct{}{}
	}

	warnings = make(map[string]error)
	for _, sp := range newPaths {
		if _, ok := oldMap[sp]; ok {
			continue
		}
		// Core dumps (.core) are often too large, do not copy them.
		// Minidumps (.dmp) are usually sufficient.
		if strings.HasSuffix(sp, ".core") {
			continue
		}

		if err := fsutil.CopyFile(sp, filepath.Join(dstDir, filepath.Base(sp))); err != nil {
			warnings[sp] = err
		}
	}
	return warnings, nil
}

// CopySystemInfo copies system information relevant to crash dumps (e.g. lsb-release) into dstDir.
func CopySystemInfo(dstDir string) error {
	return fsutil.CopyFile(lsbReleasePath, filepath.Join(dstDir, filepath.Base(lsbReleasePath)))
}
