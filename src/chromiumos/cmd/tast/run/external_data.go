// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"chromiumos/cmd/tast/logging"
)

// externalDataMap holds a mapping from data file paths used by local tests to URLs containing those files' contents.
type externalDataMap map[string]string // keys are data file paths relative to bundle dir; values are URLs

// newExternalDataMap loads an external_data.conf file at cfgPath.
func newExternalDataMap(cfgPath string) (*externalDataMap, error) {
	fileURLs, err := readExternalDataConfig(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %v: %v", cfgPath, err)
	}
	return (*externalDataMap)(&fileURLs), nil
}

// readExternalDataConfig reads a text file at p containing a mapping from data file paths to URLs.
// Each line is expected to contain:
//	- a file path (relative to the bundle dir, i.e. "<category>/data/<file>"),
//	- whitespace, and
//	- the corresponding data URL.
// Comment lines beginning with '#' are ignored.
func readExternalDataConfig(p string) (map[string]string, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	files := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			return nil, fmt.Errorf("bad line %q", line)
		}
		files[parts[0]] = parts[1]
	}
	return files, scanner.Err()
}

// localFile returns the basename of the local file storing data for the data file at p
// (which is relative to the bundle dir). The returned file is stored within the extDataDir
// passed to fetchFiles. An empty string is returned if p is not an external data file.
func (e *externalDataMap) localFile(p string) string {
	u, ok := (*e)[p]
	if !ok {
		return ""
	}
	return url.PathEscape(u)
}

// fetchFiles fetches data for the passed files, which are relative to the bundle dir.
// Files that are not registered in the map or that have already been cached are skipped.
// If lg is non-nil, it is used to log progress.
func (e *externalDataMap) fetchFiles(files []string, extDataDir string, lg logging.Logger) error {
	for _, p := range files {
		u, ok := (*e)[p]
		if !ok {
			continue
		}

		dst := filepath.Join(extDataDir, e.localFile(p))
		if _, err := os.Stat(dst); err == nil {
			continue
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}

		// TODO(derat): gsutil starts very slowly, taking about a second just to print its version.
		// Benchmark to see if it's faster to copy multiple files in parallel here.
		if lg != nil {
			lg.Debug("Downloading ", u)
		}
		if err := downloadExternalDataFile(u, dst); err != nil {
			return fmt.Errorf("failed to download %v: %v", u, err)
		}
	}
	return nil
}

// downloadExternalDataFile downloads the file at URL u to absolute path dst.
func downloadExternalDataFile(u, dst string) error {
	parsed, err := url.Parse(u)
	if err != nil {
		return err
	}
	if parsed.Path == "" {
		return errors.New("empty path")
	}
	// Permit local files for unit testing.
	if parsed.Scheme != "gs" && parsed.Scheme != "" {
		return fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
	// TODO(derat): Handle other schemes? Should match whatever Portage's SRC_URI supports.
	return exec.Command("gsutil", "-q", "cp", u, dst).Run()
}
