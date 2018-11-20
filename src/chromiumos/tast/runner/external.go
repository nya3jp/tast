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
	"sort"
	"time"

	"chromiumos/tast/devserver"
	"chromiumos/tast/testing"
)

// ExternalLinkSuffix is a file name suffix for external data link files.
// These are JSON files that can be unmarshaled into the externalLink struct.
const ExternalLinkSuffix = ".external"

// externalLink holds information of an external data link.
type externalLink struct {
	URL        string `json:"url"`
	Size       int64  `json:"size"`
	SHA256Sum  string `json:"sha256sum"`
	Executable bool   `json:"executable"`
}

// downloadJob represents a job to download an external data file and make hard links
// at several file paths.
type downloadJob struct {
	link  externalLink
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
// logs encountered errors with lf so that a single download error does not cause all tests to fail.
func processExternalDataLinks(ctx context.Context, dataDir string, tests []*testing.Test, cl devserver.Client, lf func(msg string)) {
	jobs := prepareDownloads(dataDir, tests, lf)
	if len(jobs) == 0 {
		return
	}
	runDownloads(ctx, dataDir, jobs, cl, lf)
}

// prepareDownloads computes which external data files need to be downloaded.
// It also removes stale files so they are never used even if we fail to download them later.
func prepareDownloads(dataDir string, tests []*testing.Test, lf func(msg string)) []*downloadJob {
	urlToJob := make(map[string]*downloadJob)
	hasErr := false

	reportErr := func(format string, args ...interface{}) {
		lf(fmt.Sprintf(format, args...))
		hasErr = true
	}

	for _, t := range tests {
		for _, name := range t.Data {
			destPath := filepath.Join(dataDir, t.DataDir(), name)
			linkPath := destPath + ExternalLinkSuffix

			_, err := os.Stat(linkPath)
			if os.IsNotExist(err) {
				// Not an external data file.
				continue
			} else if err != nil {
				reportErr("Failed to stat %s: %v", linkPath, err)
				continue
			}

			link, err := loadExternalLink(linkPath)
			if err != nil {
				reportErr("Failed to load %s: %v", linkPath, err)
				continue
			}

			// Decide if we need to update the destination file.
			needed := false
			f, err := os.Open(destPath)
			if err == nil {
				needed = verify(f, link) != nil
				f.Close()

				if needed {
					// Remove the stale file early so that they are never used.
					if err := os.Remove(destPath); err != nil {
						reportErr("Failed to remove stale file %s: %v", destPath, err)
						continue
					}
				}
			} else if os.IsNotExist(err) {
				needed = true
			} else {
				reportErr("Failed to stat %s: %v", destPath, err)
				continue
			}

			// To check consistency, create an entry in urlToJob even if we are not updating the destination file.
			job := urlToJob[link.URL]
			if job == nil {
				job = &downloadJob{link, nil}
				urlToJob[link.URL] = job
			} else if job.link != link {
				reportErr("Conflicting external data link found at %s: got %+v, want %+v", filepath.Join(t.DataDir(), name), link, job.link)
				continue
			}

			if needed {
				// Use O(n^2) algorithm assuming the number of duplicates is small.
				dup := false
				for _, d := range job.dests {
					if d == destPath {
						dup = true
						break
					}
				}
				if !dup {
					job.dests = append(job.dests, destPath)
				}
			}
		}
	}

	var jobs []*downloadJob
	for _, j := range urlToJob {
		if len(j.dests) > 0 {
			jobs = append(jobs, j)
		}
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].link.URL < jobs[j].link.URL
	})

	lf(fmt.Sprintf("Found %d external linked data file(s), need to download %d", len(urlToJob), len(jobs)))
	if hasErr {
		lf("Encountered some errors on scanning external data link files, but continuing anyway; corresponding tests will fail")
	}
	return jobs
}

// loadExternalLink loads a JSON file of externalLink.
func loadExternalLink(path string) (externalLink, error) {
	f, err := os.Open(path)
	if err != nil {
		return externalLink{}, err
	}
	defer f.Close()

	var link externalLink
	if err := json.NewDecoder(f).Decode(&link); err != nil {
		return externalLink{}, err
	}
	return link, nil
}

// runDownloads downloads required external data files in parallel.
func runDownloads(ctx context.Context, dataDir string, jobs []*downloadJob, cl devserver.Client, lf func(msg string)) {
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
				size, err := runDownload(ctx, dataDir, job, cl)
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
			mbs := float64(res.size) / res.duration.Seconds() / 1024 / 1024
			lf(fmt.Sprintf("Finished downloading %s (%d bytes, %v, %.1fMB/s)",
				res.url, res.size, res.duration.Round(time.Millisecond), mbs))
		}
	}
	if hasErr {
		lf("Failed to download some external data files, but continuing anyway; corresponding tests will fail")
	}
}

// runDownload downloads an external data file.
func runDownload(ctx context.Context, dataDir string, job *downloadJob, cl devserver.Client) (size int64, err error) {
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

	size, err = cl.DownloadGS(ctx, f, job.link.URL)
	if err != nil {
		return 0, err
	}

	if err := verify(f, job.link); err != nil {
		return 0, err
	}

	for _, dest := range job.dests {
		if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
			return 0, err
		}
		if err := os.Link(f.Name(), dest); err != nil {
			return 0, err
		}
	}
	return size, nil
}

// verify checks the integrity of an external data file.
func verify(f *os.File, link externalLink) error {
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if fi.Size() != link.Size {
		return fmt.Errorf("file size mismatch; got %d bytes, want %d bytes", fi.Size(), link.Size)
	}

	if _, err := f.Seek(0, 0); err != nil {
		return err
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
