// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"chromiumos/tast/testing"
)

// ExternalLinkSuffix is a file name suffix for external data link files.
// They are JSON files compatible with ExternalLink type.
const ExternalLinkSuffix = ".external-link"

// ExternalLink holds information of an external linked data.
type ExternalLink struct {
	URL        string `json:"url"`
	Size       int64  `json:"size"`
	SHA256Sum  string `json:"sha256sum"`
	Executable bool   `json:"executable"`
}

// downloadJob represents a job to download an external data and make its hard
// links at several file paths.
type downloadJob struct {
	link  ExternalLink
	dests []string
}

func resolveExternalLinks(dataDir string, tests []*testing.Test) error {
	jobs, err := computeJobs(dataDir, tests)
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		return nil
	}
	return runJobs(dataDir, jobs)
}

func computeJobs(dataDir string, tests []*testing.Test) ([]*downloadJob, error) {
	urlToJob := make(map[string]*downloadJob)

	for _, t := range tests {
		for _, name := range t.Data {
			destPath := filepath.Join(dataDir, t.DataDir(), name)
			linkPath := destPath + ExternalLinkSuffix

			linkStat, err := os.Stat(linkPath)
			if os.IsNotExist(err) {
				continue // no .external-link file
			} else if err != nil {
				return nil, err
			}

			destStat, err := os.Stat(destPath)
			if err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to stat %s: %v", destPath, err)
			}
			if err == nil && destStat.ModTime().After(linkStat.ModTime()) {
				continue // up to date
			}

			link, err := loadExternalLink(linkPath)
			if err != nil {
				return nil, fmt.Errorf("failed to load %s: %v", linkPath, err)
			}

			job := urlToJob[link.URL]
			if job == nil {
				job = &downloadJob{link, []string{destPath}}
				urlToJob[link.URL] = job
			} else if job.link != link {
				return nil, fmt.Errorf("conflicting external link declarations found for %s: %#v vs %#v", link.URL, link, job.link)
			}
			job.dests = append(job.dests, destPath)
		}
	}

	var jobs []*downloadJob
	for _, j := range urlToJob {
		jobs = append(jobs, j)
	}
	return jobs, nil
}

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

func runJobs(dataDir string, jobs []*downloadJob) error {
	jobCh := make(chan *downloadJob, len(jobs))
	for _, job := range jobs {
		jobCh <- job
	}
	close(jobCh)

	const parallelism = 4
	resCh := make(chan error, len(jobs))
	for i := 0; i < parallelism; i++ {
		go func() {
			for job := range jobCh {
				resCh <- runJob(dataDir, job)
			}
		}()
	}

	var firstErr error
	for range jobs {
		if err := <-resCh; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func runJob(dataDir string, job *downloadJob) error {
	// Create the temporary file under dataDir to make use of hard links.
	f, err := ioutil.TempFile(dataDir, ".external-download.")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()

	var mode os.FileMode = 0644
	if job.link.Executable {
		mode = 0755
	}
	if err := f.Chmod(mode); err != nil {
		return err
	}

	if err := download(f, job.link.URL); err != nil {
		return fmt.Errorf("failed to download %s: %v", job.link.URL, err)
	}

	if err := verify(f, job.link); err != nil {
		return fmt.Errorf("failed to download %s: %v", job.link.URL, err)
	}

	for _, dest := range job.dests {
		if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s: %v", dest, err)
		}
		if err := os.Link(f.Name(), dest); err != nil {
			return fmt.Errorf("failed to create a hard link at %s: %v", dest, err)
		}
	}
	return nil
}

func download(w io.Writer, url string) error {
	// TODO(nya): Use devservers.
	const gsPrefix = "gs://"
	if !strings.HasPrefix(url, gsPrefix) {
		return errors.New("only gs:// URLs are supported for now")
	}
	httpURL := "https://storage.googleapis.com/" + strings.TrimPrefix(url, gsPrefix)

	r, err := http.Get(httpURL)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	_, err = io.Copy(w, r.Body)
	return err
}

func verify(f *os.File, link ExternalLink) error {
	size, err := f.Seek(0, 2)
	if err != nil {
		return fmt.Errorf("failed to seek: %v", err)
	}
	if size != link.Size {
		return fmt.Errorf("file size mismatch; got %d bytes, want %d bytes", size, link.Size)
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
