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

	"chromiumos/cmd/tast/logging"
	"chromiumos/cmd/tast/symbolize/breakpad"
)

// Config contains parameters used when symbolizing crash files.
// This struct carries state and should be reused when symbolizing multiple crashes from the same build.
type Config struct {
	symbolDir string
	buildRoot string
	logger    logging.Logger

	unavailable breakpad.SymbolFileMap

	walkMinidump      func(path, symDir string, w io.Writer) (missing breakpad.SymbolFileMap, err error)
	downloadSymbols   func(url, destDir string, files breakpad.SymbolFileMap) (missing breakpad.SymbolFileMap, err error)
	createSymbolFiles func(cfg *Config, sf breakpad.SymbolFileMap) (missing breakpad.SymbolFileMap)
}

// symbolDir contains a directory used to store symbol files.
// buildRoot contains build root (e.g. "/build/lumpy") that produced the system image.
// If empty, inferred by extracting the board name from the minidump.
// The build root is only used if a builder path can't be extracted from the minidump.
func NewConfig(symbolDir, buildRoot string, logger logging.Logger) *Config {
	return &Config{
		symbolDir:         symbolDir,
		buildRoot:         buildRoot,
		logger:            logger,
		unavailable:       make(breakpad.SymbolFileMap),
		walkMinidump:      breakpad.WalkMinidump,
		downloadSymbols:   breakpad.DownloadSymbols,
		createSymbolFiles: createSymbolFiles,
	}
}

// SymbolizeCrash attempts to symbolize a crash file.
// path can contain either raw minidump data or a Chrome crash report.
// The (possibly-unsuccessfully-)symbolized data is written to w.
func SymbolizeCrash(path string, w io.Writer, cfg *Config) error { // NOLINT
	if cfg.symbolDir == "" {
		return errors.New("symbol directory not supplied")
	}

	dumpPath, err := getMinidumpPath(cfg, path)
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
	cfg.logger.Debugf("Got board %q and builder path %q from minidump", ri.board, ri.builderPath)
	if cfg.buildRoot == "" {
		cfg.buildRoot = filepath.Join("/build", ri.board)
	}

	cfg.logger.Debugf("Walking %v with symbol dir %v", dumpPath, cfg.symbolDir)
	b := bytes.Buffer{}
	missing, err := cfg.walkMinidump(dumpPath, cfg.symbolDir, &b)
	if err != nil {
		return fmt.Errorf("failed to walk %v: %v", dumpPath, err)
	}

	// Don't try to get any symbol files that we already established are unavailable.
	for p, id := range missing {
		if cfg.unavailable[p] == id {
			delete(missing, p)
		}
	}

	origMissingCount := len(missing)

	// FIXME: Add a config field to control whether we download or not.
	if len(missing) > 0 && ri.builderPath != "" {
		count := len(missing)
		url := breakpad.GetSymbolsURL(ri.builderPath)
		cfg.logger.Debugf("Looking for %v symbol file(s) in %v", count, url)
		if missing, err = cfg.downloadSymbols(url, cfg.symbolDir, missing); err != nil {
			cfg.logger.Logf("Failed to get symbols from %v: %v", url, err)
		} else if len(missing) == count {
			cfg.logger.Log("Didn't find any needed symbols in ", url)
		}
	}

	// Try to generate symbols from /build if we didn't download them all: https://crbug.com/904642
	if len(missing) > 0 {
		cfg.logger.Debugf("Trying to generate %v symbol file(s) from %v", len(missing), cfg.buildRoot)
		missing = cfg.createSymbolFiles(cfg, missing)
	}

	for p, id := range missing {
		cfg.unavailable[p] = id
	}

	// If we didn't write any new symbol files (possibly because there were none missing),
	// we're done -- nothing will change if we walk the minidump again.
	created := len(missing) - origMissingCount
	if created == 0 {
		_, err = io.Copy(w, &b)
		return err
	}

	// Otherwise, walk the minidump again.
	cfg.logger.Debugf("Walking %v again with %v new symbol file(s)", dumpPath, created)
	if _, err = cfg.walkMinidump(dumpPath, cfg.symbolDir, w); err != nil {
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
		cfg.logger.Debugf("Using minidump file %v", path)
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

	cfg.logger.Debugf("Writing minidump data from %v to %v", path, tf.Name())
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
