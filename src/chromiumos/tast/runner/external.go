// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"chromiumos/tast/devserver"
	"chromiumos/tast/testing"
)

// ExternalLinkSuffix is a file name suffix for external data link files.
// They are JSON files compatible with ExternalLink type.
const ExternalLinkSuffix = ".external"

// ExternalLink holds information of an external data link.
type ExternalLink struct {
	URL        string `json:"url"`
	Size       int64  `json:"size"`
	SHA256Sum  string `json:"sha256sum"`
	Executable bool   `json:"executable"`
}

// downloadJob represents a job to download an external data and make hard links
// at several file paths.
type downloadJob struct {
	link  ExternalLink
	dests []string
}

// downloadResult represents a result of a downloadJob.
type downloadResult struct {
	url      string
	duration time.Duration
	size     int64
	err      error
}

// processExternalDataLinks downloads missing or stale external data files associated with tests.
// This function does not return errors; instead it tries to download files as far as possible and
// logs encountered errors with lf so that single download error does not cause all tests to fail.
func processExternalDataLinks(dataDir string, tests []*testing.Test, cl devserver.Client, lf func(msg string)) {
	jobs := prepareDownloads(dataDir, tests, lf)
	if len(jobs) == 0 {
		return
	}
	runDownloads(dataDir, jobs, cl, lf)
}

// prepareDownloads computes which external data files need to be downloaded.
// It also removes stale files so they are never used even if we fail to download them later.
func prepareDownloads(dataDir string, tests []*testing.Test, lf func(msg string)) []*downloadJob {
	urlToJob := make(map[string]*downloadJob)
	hasErr := false

	for _, t := range tests {
		for _, name := range t.Data {
			destPath := filepath.Join(dataDir, t.DataDir(), name)
			linkPath := destPath + ExternalLinkSuffix

			linkStat, err := os.Stat(linkPath)
			if os.IsNotExist(err) {
				// Not an external data.
				continue
			} else if err != nil {
				lf(fmt.Sprintf("Failed to stat %s: %v", linkPath, err))
				hasErr = true
				continue
			}

			destStat, err := os.Stat(destPath)
			if err != nil && !os.IsNotExist(err) {
				lf(fmt.Sprintf("Failed to stat %s: %v", destPath, err))
				hasErr = true
				continue
			}

			stale := os.IsNotExist(err) || linkStat.ModTime().After(destStat.ModTime())
			if stale && !os.IsNotExist(err) {
				// Remove stale files early so that they are never used.
				if err := os.Remove(destPath); err != nil {
					lf(fmt.Sprintf("Failed to remove a stale file %s: %v", destPath, err))
					hasErr = true
					continue
				}
			}

			link, err := loadExternalLink(linkPath)
			if err != nil {
				lf(fmt.Sprintf("Failed to load %s: %v", linkPath, err))
				hasErr = true
				continue
			}

			// To check consistency, create an entry in urlToJob even if the file is stale.
			job := urlToJob[link.URL]
			if job == nil {
				job = &downloadJob{link, nil}
				urlToJob[link.URL] = job
			} else if job.link != link {
				lf(fmt.Sprintf("Conflicting external data link found at %s: got %+v, want %+v", filepath.Join(t.DataDir(), name), link, job.link))
				hasErr = true
				continue
			}

			if stale {
				job.dests = append(job.dests, destPath)
			}
		}
	}

	var jobs []*downloadJob
	for _, j := range urlToJob {
		if len(j.dests) > 0 {
			jobs = append(jobs, j)
		}
	}

	lf(fmt.Sprintf("Found %d external linked data, need to download %d", len(urlToJob), len(jobs)))
	if hasErr {
		lf("Encountered some errors on scanning external data link files, but continuing anyway; corresponding tests will fail")
	}
	return jobs
}

// loadExternalLink loads a JSON file of ExternalLink.
func loadExternalLink(path string) (ExternalLink, error) {
	f, err := os.Open(path)
	if err != nil {
		return ExternalLink{}, err
	}
	defer f.Close()

	var link ExternalLink
	if err := json.NewDecoder(f).Decode(&link); err != nil {
		return ExternalLink{}, err
	}
	return link, nil
}

// runDownloads downloads required external data files in parallel.
func runDownloads(dataDir string, jobs []*downloadJob, cl devserver.Client, lf func(msg string)) {
	jobCh := make(chan *downloadJob, len(jobs))
	for _, job := range jobs {
		jobCh <- job
	}
	close(jobCh)

	const parallelism = 4
	resCh := make(chan *downloadResult, len(jobs))
	for i := 0; i < parallelism; i++ {
		go func() {
			for job := range jobCh {
				lf(fmt.Sprintf("Downloading %s", job.link.URL))
				start := time.Now()
				size, err := runDownload(dataDir, job, cl)
				duration := time.Since(start)
				resCh <- &downloadResult{job.link.URL, duration, size, err}
			}
		}()
	}

	hasErr := false
	for range jobs {
		res := <-resCh
		if res.err != nil {
			lf(fmt.Sprintf("Failed to download %s: %v", res.url, res.err))
			hasErr = true
		} else {
			secs := res.duration.Seconds()
			mbs := float64(res.size) / secs / 1024 / 1024
			lf(fmt.Sprintf("Finished downloading %s (%d bytes, %.0fs, %.1fMB/s)", res.url, res.size, secs, mbs))
		}
	}
	if hasErr {
		lf("Failed to download some external data files, but continuing anyway; corresponding tests will fail")
	}
}

// runDownload downloads an external data file.
func runDownload(dataDir string, job *downloadJob, cl devserver.Client) (size int64, err error) {
	// Create the temporary file under dataDir to make use of hard links.
	f, err := ioutil.TempFile(dataDir, ".external-download.")
	if err != nil {
		return 0, err
	}
	defer os.Remove(f.Name())
	defer f.Close()

	var mode os.FileMode = 0644
	if job.link.Executable {
		mode = 0755
	}
	if err := f.Chmod(mode); err != nil {
		return 0, err
	}

	// TODO(nya): Consider applying timeout.
	size, err = cl.DownloadGS(context.Background(), f, job.link.URL)
	if err != nil {
		return 0, err
	}

	if err := verify(f, job.link); err != nil {
		return 0, err
	}

	for _, dest := range job.dests {
		if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
			return 0, fmt.Errorf("failed to remove %s: %v", dest, err)
		}
		if err := os.Link(f.Name(), dest); err != nil {
			return 0, fmt.Errorf("failed to create a hard link at %s: %v", dest, err)
		}
	}
	return size, nil
}

// verify checks integrity of a downloaded file.
func verify(f *os.File, link ExternalLink) error {
	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("failed to seek: %v", err)
	}
	if fi.Size() != link.Size {
		return fmt.Errorf("file size mismatch; got %d bytes, want %d bytes", fi.Size(), link.Size)
	}

	if _, err := f.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to seek: %v", err)
	}

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return fmt.Errorf("failed to compute hash: %v", err)
	}
	hash := hex.EncodeToString(hasher.Sum(nil))
	if hash != link.SHA256Sum {
		return fmt.Errorf("hash mismatch; got %s, want %s", hash, link.SHA256Sum)
	}
	return nil
}
