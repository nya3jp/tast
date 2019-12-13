// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package breakpad

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	imageArchiveBaseURL   = "gs://chromeos-image-archive" // contains build artifacts
	imageArchiveFilename  = "debug_breakpad.tar.xz"       // filename within builder path
	imageArchiveTarPrefix = "debug/breakpad/"             // prefix for symbol dir in .tar.xz
)

// GetSymbolsURL returns the Cloud Storage URL of the .tar.xz file containing Breakpad
// debug symbols for builderPath (e.g. "cave-release/R65-10286.0.0").
func GetSymbolsURL(builderPath string) string {
	return fmt.Sprintf("%s/%s/%s", imageArchiveBaseURL, builderPath, imageArchiveFilename)
}

// DownloadSymbols downloads url (see GetSymbolsURL) and extracts the symbol files specified
// in files to destDir. The number of files that were created is returned.
func DownloadSymbols(url, destDir string, files SymbolFileMap) (created int, err error) {
	// Create a set of relative symbol file paths.
	wanted := make(map[string]struct{}, len(files))
	for p, id := range files {
		wanted[GetSymbolFilePath("", filepath.Base(p), id)] = struct{}{}
	}

	as, err := newArchiveStreamer(url)
	if err != nil {
		return 0, err
	}
	defer as.close()

	tr := tar.NewReader(as.out)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return created, err
		}

		// Strip off the weird leading directories used in archive files and check if this
		// is one of the files we're looking for.
		p := hdr.Name[len(imageArchiveTarPrefix):]
		if _, ok := wanted[p]; !ok || hdr.Typeflag != tar.TypeReg {
			continue
		}

		// tar.Reader functions as an io.Reader and returns data from the current entry
		// until Next is called.
		if err := writeSymbolFile(filepath.Join(destDir, p), tr); err != nil {
			return created, err
		}

		created++
		delete(wanted, p)
		if len(wanted) == 0 {
			break
		}
	}

	return created, nil
}

// writeSymbolFiles creates a new file (including parent directory) at p
// and copies data from r into it until io.EOF is reached.
func writeSymbolFile(p string, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}

	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, r)
	return err
}

// archiveStreamer uses gsutil to read a file and xz to decompress its contents.
type archiveStreamer struct {
	out    io.Reader // provides uncompressed data
	gs, xz *exec.Cmd // gsutil and xz commands
}

// newArchiveStreamer starts and returns a new archiveStreamer that decompresses
// the xz-compressed file at src.
func newArchiveStreamer(src string) (*archiveStreamer, error) {
	// TODO(derat): If google-cloud-go is packaged at some point, consider trying to
	// use it instead of gsutil. The Go standard library doesn't support xz, though, so
	// some of this will need to happen out of process regardless.
	as := archiveStreamer{
		gs: exec.Command("gsutil", "cp", src, "-"),
		xz: exec.Command("xz", "-d"),
	}

	var err error
	if as.xz.Stdin, err = as.gs.StdoutPipe(); err != nil {
		return nil, err
	}
	if as.out, err = as.xz.StdoutPipe(); err != nil {
		return nil, err
	}

	if err = as.xz.Start(); err != nil {
		return nil, err
	}
	if err = as.gs.Start(); err != nil {
		as.close() // kill previously-started xz process
		return nil, err
	}
	return &as, nil
}

// close kills the gsutil and xz processes if they're still running and lets
// the system reclaim their resources.
func (as *archiveStreamer) close() error {
	var gserr, xzerr error
	if as.gs.Process != nil {
		as.gs.Process.Kill()
		_, gserr = as.gs.Process.Wait()
	}
	if as.xz.Process != nil {
		as.xz.Process.Kill()
		_, xzerr = as.xz.Process.Wait()
	}
	if gserr != nil {
		return gserr
	}
	return xzerr
}
