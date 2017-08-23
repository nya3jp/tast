// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package host

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// getLocalSHA1s returns SHA1s for files in paths.
// An error is returned if any files are missing.
func getLocalSHA1s(paths []string) (map[string]string, error) {
	sums := make(map[string]string)

	for _, p := range paths {
		if fi, err := os.Stat(p); err != nil {
			return nil, err
		} else if fi.IsDir() {
			// Use a bogus hash for directories to ensure they're copied.
			sums[p] = localDirHash
			continue
		}

		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		h := sha1.New()
		if _, err := io.Copy(h, f); err != nil {
			return nil, err
		}
		sums[p] = hex.EncodeToString(h.Sum(nil))
	}

	return sums, nil
}

// getRemoteSHA1s returns SHA1s for the files paths on h.
// Missing files are excluded from the returned map.
func getRemoteSHA1s(ctx context.Context, h Host, paths []string) (map[string]string, error) {
	cmd := "sha1sum"
	for _, p := range paths {
		cmd += " " + QuoteShellArg(p)
	}
	// TODO(derat): Find a classier way to ignore missing files.
	cmd += " 2>/dev/null || true"

	out, err := h.Run(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to hash files: %v", err)
	}

	sums := make(map[string]string)
	for _, l := range strings.Split(string(out), "\n") {
		if l == "" {
			continue
		}
		f := strings.Fields(l)
		if len(f) != 2 {
			return nil, fmt.Errorf("unexpected line %q from sha1sum", l)
		}
		sums[f[1]] = f[0]
	}
	return sums, nil
}

// findChangedFiles returns paths from files that differ between ldir on the local
// machine and rdir on h. This function is intended for use when pushing files to h;
// an error is returned if one or more files are missing locally, but not if they're
// only missing remotely. Local directories are always listed as having been changed.
func findChangedFiles(ctx context.Context, h Host, ldir, rdir string, files []string) ([]string, error) {
	if len(files) == 0 {
		return []string{}, nil
	}

	// TODO(derat): For large binary files, it may be faster to do an extra round trip first
	// to get file sizes. If they're different, there's no need to spend the time and
	// CPU to run sha1sum.
	lp := make([]string, len(files))
	rp := make([]string, len(files))
	for i, f := range files {
		lp[i] = filepath.Join(ldir, f)
		rp[i] = filepath.Join(rdir, f)
	}

	var err error
	var lh, rh map[string]string
	if lh, err = getLocalSHA1s(lp); err != nil {
		return nil, fmt.Errorf("failed to get SHA1s of local file(s): %v", err)
	}
	if rh, err = getRemoteSHA1s(ctx, h, rp); err != nil {
		return nil, fmt.Errorf("failed to get SHA1s of remote file(s): %v", err)
	}

	cf := make([]string, 0)
	for i, f := range files {
		// TODO(derat): Also check modes, maybe.
		if lh[lp[i]] != rh[rp[i]] {
			cf = append(cf, f)
		}
	}
	return cf, nil
}
