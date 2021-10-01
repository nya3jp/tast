// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package symbolize provides support for symbolizing crashes.
package symbolize

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"chromiumos/tast/cmd/tast/internal/symbolize/breakpad"
	"chromiumos/tast/internal/logging"
)

// Config contains parameters used when symbolizing crash files.
type Config struct {
	// SymbolDir contains a directory used to store symbol files.
	SymbolDir string
	// BuilderPath, for example, "betty-release/R91-13892.0.0", identifies the location
	// of debug symbols in gs://chromeos-image-archive.
	BuilderPath string
	// BuildRoot contains build root (e.g. "/build/lumpy") that produced the system image.
	// If empty, inferred by extracting the board name from the minidump.
	// The build root is only used if a builder path can't be extracted from the minidump.
	BuildRoot string
}

// SymbolizeCrash attempts to symbolize a crash file.
// path can contain either raw minidump data or a Chrome crash report.
// The (possibly-unsuccessfully-)symbolized data is written to w.
func SymbolizeCrash(ctx context.Context, path string, w io.Writer, cfg Config) error { // NOLINT
	if cfg.SymbolDir == "" {
		return errors.New("symbol directory not supplied")
	}

	dumpPath, err := getMinidumpPath(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to get minidump: %v", err)
	}
	// If we created a temporary file, delete it later.
	if dumpPath != path {
		defer os.Remove(dumpPath)
	}

	ri, err := getMinidumpReleaseInfo(ctx, dumpPath)
	if err != nil {
		return fmt.Errorf("failed to get release info from %v: %v", dumpPath, err)
	}
	if !ri.hasBuildInfo() && cfg.BuildRoot == "" && cfg.BuilderPath == "" {
		return errors.New("minidump does not contain release info, please supply --builderpath or --buildroot parameter to fix this error")
	}
	logging.Debugf(ctx, "Got board %q and builder path %q from minidump", ri.board, ri.builderPath)
	if cfg.BuildRoot == "" {
		cfg.BuildRoot = filepath.Join("/build", ri.board)
	}
	if cfg.BuilderPath == "" {
		cfg.BuilderPath = ri.builderPath
	}

	logging.Debugf(ctx, "Walking %v with symbol dir %v", dumpPath, cfg.SymbolDir)
	b := bytes.Buffer{}
	missing, err := breakpad.WalkMinidump(dumpPath, cfg.SymbolDir, &b)
	if err != nil {
		return fmt.Errorf("failed to walk %v: %v", dumpPath, err)
	}

	created := 0
	if len(missing) > 0 {
		if cfg.BuilderPath != "" {
			url := breakpad.GetSymbolsURL(cfg.BuilderPath)
			logging.Debugf(ctx, "Extracting %v symbol file(s) from %v", len(missing), url)
			if created, err = breakpad.DownloadSymbols(url, cfg.SymbolDir, missing); err != nil {
				// Keep going so we can print what we have.
				logging.Infof(ctx, "Failed to get symbols from %v: %v", url, err)
			}
			if ri.lacrosVersion != "" {
				lacrosURL := breakpad.GetLacrosSymbolsURL(ri.lacrosVersion)
				logging.Debugf(ctx, "Extracting Lacros symbols from %v", lacrosURL)
				err := breakpad.DownloadLacrosSymbols(lacrosURL, cfg.SymbolDir)
				if err != nil {
					return fmt.Errorf("cannot obtain Lacros symbols: %v", err)
				}
				// DownloadLacrosSymbols creates exactly one file.
				created++
			}
		} else {
			logging.Debugf(ctx, "Generating %v symbol file(s) from %v", len(missing), cfg.BuildRoot)
			created = createSymbolFiles(ctx, &cfg, missing)
		}
	}

	// If we didn't write any new symbol files (possibly because there were none missing),
	// we're done -- nothing will change if we walk the minidump again.
	if created == 0 {
		_, err = io.Copy(w, &b)
		return err
	}

	// Otherwise, walk the minidump again.
	logging.Debugf(ctx, "Walking %v again with %v new symbol file(s)", dumpPath, created)
	if _, err = breakpad.WalkMinidump(dumpPath, cfg.SymbolDir, w); err != nil {
		return fmt.Errorf("failed to re-walk %v: %v", dumpPath, err)
	}
	return nil
}

// getMinidumpPath returns the path to a file containing minidump data from path.
// If path contains raw minidump data, it will be returned directly.
// If path contains a Chrome crash report, its minidump data will be written to a temporary file.
func getMinidumpPath(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// If this is a regular minidump file, we're done.
	if isDump, err := breakpad.IsMinidump(f); err != nil {
		return "", err
	} else if isDump {
		logging.Debugf(ctx, "Using minidump file %v", path)
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

	logging.Debugf(ctx, "Writing minidump data from %v to %v", path, tf.Name())
	if _, err = io.CopyN(tf, f, int64(dumpLen)); err != nil {
		os.Remove(tf.Name())
		return "", err
	}
	return tf.Name(), nil
}

// getMinidumpReleaseInfo returns release information contained in the minidump file at path.
func getMinidumpReleaseInfo(ctx context.Context, path string) (*releaseInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := breakpad.GetMinidumpReleaseInfo(f)
	if err != nil {
		return nil, err
	}

	// We do not expect minidumps with both /etc/lsb-release and Crashpad
	// annotations, but will log both if given such a file.
	found := false
	if data.EtcLsbRelease != "" {
		found = true
		logging.Debug(ctx, "Found /etc/lsb-release.")
	}
	if data.CrashpadAnnotations != nil {
		found = true
		// Crashpad annotation may or may not contains board and builder path, so print them.
		logging.Debugf(ctx, "Found Crashpad annotations: %v", data.CrashpadAnnotations)
	}
	if !found {
		logging.Debug(ctx, "Minidump does not contain /etc/lsb-release or Crashpad annotations.")
	}

	return getReleaseInfo(data)
}
