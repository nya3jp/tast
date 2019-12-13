// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package symbolize provides support for symbolizing crashes.
package symbolize

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"chromiumos/tast/cmd/tast/logging"
	"chromiumos/tast/cmd/tast/symbolize/breakpad"
)

// Config contains parameters used when symbolizing crash files.
type Config struct {
	// Logger is used to log progress and errors.
	Logger logging.Logger
	// SymbolDir contains a directory used to store symbol files.
	SymbolDir string
	// BuildRoot contains build root (e.g. "/build/lumpy") that produced the system image.
	// If empty, inferred by extracting the board name from the minidump.
	// The build root is only used if a builder path can't be extracted from the minidump.
	BuildRoot string
}

// SymbolizeCrash attempts to symbolize a crash file.
// path can contain either raw minidump data or a Chrome crash report.
// The (possibly-unsuccessfully-)symbolized data is written to w.
func SymbolizeCrash(path string, w io.Writer, cfg Config) error { // NOLINT
	if cfg.SymbolDir == "" {
		return errors.New("symbol directory not supplied")
	}

	dumpPath, err := getMinidumpPath(&cfg, path)
	if err != nil {
		return fmt.Errorf("failed to get minidump: %v", err)
	}
	// If we created a temporary file, delete it later.
	if dumpPath != path {
		defer os.Remove(dumpPath)
	}

	ri, err := getMinidumpReleaseInfo(dumpPath)
	if err != nil {
		return fmt.Errorf("failed to get release info from %v: %v", dumpPath, err)
	}
	cfg.Logger.Debugf("Got board %q and builder path %q from minidump", ri.board, ri.builderPath)
	if cfg.BuildRoot == "" {
		cfg.BuildRoot = filepath.Join("/build", ri.board)
	}

	cfg.Logger.Debugf("Walking %v with symbol dir %v", dumpPath, cfg.SymbolDir)
	b := bytes.Buffer{}
	missing, err := breakpad.WalkMinidump(dumpPath, cfg.SymbolDir, &b)
	if err != nil {
		return fmt.Errorf("failed to walk %v: %v", dumpPath, err)
	}

	created := 0
	if len(missing) > 0 {
		if ri.builderPath != "" {
			url := breakpad.GetSymbolsURL(ri.builderPath)
			cfg.Logger.Debugf("Extracting %v symbol file(s) from %v", len(missing), url)
			if created, err = breakpad.DownloadSymbols(url, cfg.SymbolDir, missing); err != nil {
				// Keep going so we can print what we have.
				cfg.Logger.Logf("Failed to get symbols from %v: %v", url, err)
			}
		} else {
			cfg.Logger.Debugf("Generating %v symbol file(s) from %v", len(missing), cfg.BuildRoot)
			created = createSymbolFiles(&cfg, missing)
		}
	}

	// If we didn't write any new symbol files (possibly because there were none missing),
	// we're done -- nothing will change if we walk the minidump again.
	if created == 0 {
		_, err = io.Copy(w, &b)
		return err
	}

	// Otherwise, walk the minidump again.
	cfg.Logger.Debugf("Walking %v again with %v new symbol file(s)", dumpPath, created)
	if _, err = breakpad.WalkMinidump(dumpPath, cfg.SymbolDir, w); err != nil {
		return fmt.Errorf("failed to re-walk %v: %v", dumpPath, err)
	}
	return nil
}

// getMinidumpPath returns the path to a file containing minidump data from path.
// If path contains raw minidump data, it will be returned directly.
// If path contains a Chrome crash report, its minidump data will be written to a temporary file.
func getMinidumpPath(cfg *Config, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// If this is a regular minidump file, we're done.
	if isDump, err := breakpad.IsMinidump(f); err != nil {
		return "", err
	} else if isDump {
		cfg.Logger.Debugf("Using minidump file %v", path)
		return path, nil
	}

	// Otherwise, check if this is a Chrome crash report.
	if _, err = f.Seek(0, 0); err != nil {
		return "", err
	}
	var dumpOffset, dumpLen int
	if _, dumpOffset, dumpLen, err = breakpad.ReadCrashReport(f); err != nil {
		return "", err
	}

	// Copy the minidump data to a temp file.
	if _, err = f.Seek(int64(dumpOffset), 0); err != nil {
		return "", err
	}
	tf, err := ioutil.TempFile("", "tast_"+filepath.Base(path)+".")
	if err != nil {
		return "", err
	}
	defer tf.Close()

	cfg.Logger.Debugf("Writing minidump data from %v to %v", path, tf.Name())
	if _, err = io.CopyN(tf, f, int64(dumpLen)); err != nil {
		os.Remove(tf.Name())
		return "", err
	}
	return tf.Name(), nil
}

// getMinidumpReleaseInfo returns release information contained in the minidump file at path.
func getMinidumpReleaseInfo(path string) (*releaseInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := breakpad.GetMinidumpReleaseInfo(f)
	if err != nil {
		return nil, err
	}
	return getReleaseInfo(data), nil
}
