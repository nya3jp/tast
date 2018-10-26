// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package crash can be used by local tests to interact with on-device crash reports.
package crash

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DefaultCrashDir contains the directory where the kernel writes core and minidump files.
	DefaultCrashDir = "/var/spool/crash"
	// ChromeCrashDir contains the directory where Chrome writes minidump files.
	// Tests configure Chrome to write crashes to this location/ by setting the BREAKPAD_DUMP_LOCATION
	// environment variable. This overrides the default /home/chronos/user/crash location, which is in
	// the user's cryptohome and hence is only accessible while they are logged in.
	ChromeCrashDir = "/home/chronos/crash"

	// CoreExt is the extension for core files.
	CoreExt = ".core"
	// MinidumpExt is the extension for minidump crash files.
	MinidumpExt = ".dmp"
	// LogExt is the extension for log files containing additional information that are written by crash_reporter.
	LogExt = ".log"
	// MetadataExt is the extension for metadata files written by crash collectors and read by crash_sender.
	MetadataExt = ".meta"

	lsbReleasePath = "/etc/lsb-release"
)

// copyFile creates a new file at dst containing the contents of the file at src.
func copyFile(dst, src string) error {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()

	df, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer df.Close()

	_, err = io.Copy(df, sf)
	return err
}

// DefaultDirs returns all standard directories to which crashes are written.
func DefaultDirs() []string {
	return []string{DefaultCrashDir, ChromeCrashDir}
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
			ext := filepath.Ext(fn)
			if ext == CoreExt || ext == MinidumpExt || ext == LogExt || ext == MetadataExt {
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
func CopyNewFiles(dstDir string, newPaths, oldPaths []string, maxPerExec int) (
	warnings map[string]error, err error) {
	oldMap := make(map[string]struct{})
	for _, p := range oldPaths {
		oldMap[p] = struct{}{}
	}

	warnings = make(map[string]error)
	execCount := make(map[string]int)
	for _, sp := range newPaths {
		if _, ok := oldMap[sp]; ok {
			continue
		}

		var base string
		if parts := strings.Split(filepath.Base(sp), "."); len(parts) > 2 {
			// If there are at least three components in the crash filename, assume
			// that it's something like name.id.dmp and count the first part.
			base = filepath.Join(filepath.Dir(sp), parts[0])
		} else {
			// Otherwise, add it to the per-directory count.
			base = filepath.Dir(sp)
		}
		if maxPerExec > 0 && execCount[base] == maxPerExec {
			warnings[sp] = errors.New("skipping; too many files")
			continue
		}

		if err := copyFile(filepath.Join(dstDir, filepath.Base(sp)), sp); err != nil {
			warnings[sp] = err
		} else {
			execCount[base]++
		}
	}
	return warnings, nil
}

// CopySystemInfo copies system information relevant to crash dumps (e.g. lsb-release) into dstDir.
func CopySystemInfo(dstDir string) error {
	return copyFile(filepath.Join(dstDir, filepath.Base(lsbReleasePath)), lsbReleasePath)
}
