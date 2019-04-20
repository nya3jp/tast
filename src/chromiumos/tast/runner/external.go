// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"chromiumos/tast/devserver"
	"chromiumos/tast/testing"
)

// externalLinkType represents a type of an external data link.
type externalLinkType string

const (
	// typeStatic is for a link to a file on web with fixed URL and content.
	typeStatic externalLinkType = ""

	// typeArtifact is for a link to a file in Chrome OS build artifacts
	// corresponding to the DUT image version.
	typeArtifact = "artifact"
)

// externalLink holds information of an external data link.
type externalLink struct {
	// Type declares the type of the external data link.
	Type externalLinkType `json:"type"`

	// StaticURL is the URL of the static external data file on Google Cloud Storage.
	// This field is valid for static external data links only.
	StaticURL string `json:"url"`

	// Size is the size of the external data file in bytes.
	// This field is valid for static external data links only.
	Size int64 `json:"size"`

	// Size is SHA256 hash of the external data file.
	// This field is valid for static external data links only.
	SHA256Sum string `json:"sha256sum"`

	// Name is the file name of a build artifact.
	// This field is valid for build artifact external data links only.
	Name string `json:"name"`

	// Executable specifies whether the external data file is executable.
	// If this is true, executable permission is given to the downloaded file.
	Executable bool `json:"executable"`

	// computedURL is the URL of the external data file on Google Cloud Storage.
	// This field is filled by Finalize.
	computedURL string
}

// Finalize checks if the link definition is correct, and fills extra fields.
func (l *externalLink) Finalize(artifactsURL string) error {
	switch l.Type {
	case typeStatic:
		if l.StaticURL == "" {
			return errors.New("url field must not be empty for static external data file")
		}
		if l.Name != "" {
			return errors.New("name field must be empty for static external data file")
		}
		if l.SHA256Sum == "" {
			return errors.New("sha256sum field must not be empty for static external data file")
		}
		l.computedURL = l.StaticURL
	case typeArtifact:
		if l.StaticURL != "" {
			return errors.New("url field must be empty for artifact external data file")
		}
		if l.Name == "" {
			return errors.New("name field must not be empty for artifact external data file")
		}
		if l.SHA256Sum != "" {
			return errors.New("sha256sum field must be empty for artifact external data file")
		}
		if l.Size != 0 {
			return errors.New("size field must be empty for artifact external data file")
		}
		if artifactsURL == "" {
			return errors.New("build artifact URL is unknown (running a developer build?)")
		}
		l.computedURL = artifactsURL + l.Name
	default:
		return fmt.Errorf("unknown external data link type %q", l.Type)
	}
	return nil
}

// downloadJob represents a job to download an external data file and make hard links
// at several file paths.
type downloadJob struct {
	link  externalLink
	dests []string
}

// downloadResult represents a result of a downloadJob.
type downloadResult struct {
	job      *downloadJob
	duration time.Duration
	size     int64
	err      error
}

// processExternalDataLinks downloads missing or stale external data files associated with tests.
// dataDir is the path to the base directory containing external data link files (typically
// "/usr/local/share/tast/data" on DUT). artifactURL is the URL of Google Cloud Storage directory,
// ending with a slash, containing build artifacts for the current Chrome OS image.
// This function does not return errors; instead it tries to download files as far as possible and
// logs encountered errors with lf so that a single download error does not cause all tests to fail.
func processExternalDataLinks(ctx context.Context, dataDir, artifactsURL string, tests []*testing.Test, cl devserver.Client, lf func(msg string)) {
	jobs := prepareDownloads(dataDir, artifactsURL, tests, lf)
	if len(jobs) == 0 {
		return
	}
	runDownloads(ctx, dataDir, jobs, cl, lf)
}

// prepareDownloads computes which external data files need to be downloaded.
// It also removes stale files so they are never used even if we fail to download them later.
// When it encounters errors, *.external-error files are saved so that they can be read and
// reported by bundles later.
func prepareDownloads(dataDir, artifactsURL string, tests []*testing.Test, lf func(msg string)) []*downloadJob {
	urlToJob := make(map[string]*downloadJob)
	hasErr := false

	for _, t := range tests {
		for _, name := range t.Data {
			destPath := filepath.Join(dataDir, t.DataDir(), name)
			linkPath := destPath + testing.ExternalLinkSuffix
			errorPath := destPath + testing.ExternalErrorSuffix

			reportErr := func(format string, args ...interface{}) {
				msg := fmt.Sprintf("failed to prepare downloading %s: %s", name, fmt.Sprintf(format, args...))
				lf(strings.ToUpper(msg[:1]) + msg[1:])
				ioutil.WriteFile(errorPath, []byte(msg), 0666)
				hasErr = true
			}

			// Clear the error message first.
			os.Remove(errorPath)

			_, err := os.Stat(linkPath)
			if os.IsNotExist(err) {
				// Not an external data file.
				continue
			} else if err != nil {
				reportErr("failed to stat %s: %v", linkPath, err)
				continue
			}

			link, err := loadExternalLink(linkPath, artifactsURL)
			if err != nil {
				reportErr("failed to load %s: %v", linkPath, err)
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
						reportErr("failed to remove stale file %s: %v", destPath, err)
						continue
					}
				}
			} else if os.IsNotExist(err) {
				needed = true
			} else {
				reportErr("failed to stat %s: %v", destPath, err)
				continue
			}

			// To check consistency, create an entry in urlToJob even if we are not updating the destination file.
			job := urlToJob[link.computedURL]
			if job == nil {
				job = &downloadJob{link, nil}
				urlToJob[link.computedURL] = job
			} else if job.link != link {
				reportErr("conflicting external data link found at %s: got %+v, want %+v", filepath.Join(t.DataDir(), name), link, job.link)
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
		return jobs[i].link.computedURL < jobs[j].link.computedURL
	})

	lf(fmt.Sprintf("Found %d external linked data file(s), need to download %d", len(urlToJob), len(jobs)))
	if hasErr {
		lf("Encountered some errors on scanning external data link files, but continuing anyway; corresponding tests will fail")
	}
	return jobs
}

// loadExternalLink loads a JSON file of externalLink.
func loadExternalLink(path, artifactsURL string) (externalLink, error) {
	f, err := os.Open(path)
	if err != nil {
		return externalLink{}, err
	}
	defer f.Close()

	var link externalLink
	if err := json.NewDecoder(f).Decode(&link); err != nil {
		return externalLink{}, err
	}

	if err := link.Finalize(artifactsURL); err != nil {
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
				lf("Downloading " + job.link.computedURL)
				start := time.Now()
				size, err := runDownload(ctx, dataDir, job, cl)
				duration := time.Since(start)
				resCh <- &downloadResult{job, duration, size, err}
			}
		}()
	}

	hasErr := false
	finished := 0
	for finished < len(jobs) {
		select {
		case res := <-resCh:
			if res.err != nil {
				msg := fmt.Sprintf("failed to download %s: %v", res.job.link.computedURL, res.err)
				lf(strings.ToUpper(msg[:1]) + msg[1:])
				for _, dest := range res.job.dests {
					ioutil.WriteFile(dest+testing.ExternalErrorSuffix, []byte(msg), 0666)
				}
				hasErr = true
			} else {
				mbs := float64(res.size) / res.duration.Seconds() / 1024 / 1024
				lf(fmt.Sprintf("Finished downloading %s (%d bytes, %v, %.1fMB/s)",
					res.job.link.computedURL, res.size, res.duration.Round(time.Millisecond), mbs))
			}
			finished++
		case <-time.After(30 * time.Second):
			// Without this keep-alive message, the tast command may think that the SSH connection was lost.
			// TODO(nya): Remove this keep-alive logic after 20190701.
			lf("Still downloading...")
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

	size, err = cl.DownloadGS(ctx, f, job.link.computedURL)
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
	if link.Type == typeArtifact {
		// For artifacts, we do not verify files.
		return nil
	}

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
