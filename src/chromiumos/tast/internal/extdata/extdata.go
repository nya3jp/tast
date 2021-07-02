// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package extdata implements the external data file mechanism.
package extdata

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
	"reflect"
	"sort"
	"strings"
	"time"

	"chromiumos/tast/internal/devserver"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
)

// LinkType represents a type of an external data link.
type LinkType string

const (
	// TypeStatic is for a link to a file on web with fixed URL and content.
	TypeStatic LinkType = ""

	// TypeArtifact is for a link to a file in Chrome OS build artifacts
	// corresponding to the DUT image version.
	TypeArtifact LinkType = "artifact"
)

// LinkData defines the schema of external data link files.
type LinkData struct {
	// Type declares the type of the external data link.
	Type LinkType `json:"type"`

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
}

// link holds information of an external data link.
type link struct {
	// Data holds the original LinkData.
	Data LinkData

	// ComputedURL is the URL of the external data file on Google Cloud Storage.
	ComputedURL string
}

// newLink creates link from LinkData.
func newLink(d *LinkData, artifactsURL string) (*link, error) {
	switch d.Type {
	case TypeStatic:
		if d.StaticURL == "" {
			return nil, errors.New("url field must not be empty for static external data file")
		}
		if d.Name != "" {
			return nil, errors.New("name field must be empty for static external data file")
		}
		if d.SHA256Sum == "" {
			return nil, errors.New("sha256sum field must not be empty for static external data file")
		}
		return &link{Data: *d, ComputedURL: d.StaticURL}, nil
	case TypeArtifact:
		if d.StaticURL != "" {
			return nil, errors.New("url field must be empty for artifact external data file")
		}
		if d.Name == "" {
			return nil, errors.New("name field must not be empty for artifact external data file")
		}
		if d.SHA256Sum != "" {
			return nil, errors.New("sha256sum field must be empty for artifact external data file")
		}
		if d.Size != 0 {
			return nil, errors.New("size field must be empty for artifact external data file")
		}
		if artifactsURL == "" {
			return nil, errors.New("build artifact URL is unknown (running a developer build?)")
		}
		return &link{Data: *d, ComputedURL: artifactsURL + d.Name}, nil
	default:
		return nil, fmt.Errorf("unknown external data link type %q", d.Type)
	}
}

// DownloadJob represents a job to download an external data file and make hard links
// at several file paths.
type DownloadJob struct {
	link  *link
	dests []string
}

// downloadResult represents a result of a DownloadJob.
type downloadResult struct {
	job      *DownloadJob
	duration time.Duration
	size     int64
	err      error
}

// Manager manages operations for external data files.
type Manager struct {
	dataDir      string
	artifactsURL string
	all          []string // all the locations external data files can exist.
	// inuse is a mutable field that maps external data files to the number
	// of entities currently using it.
	inuse map[string]int
}

// NewManager creates a new Manager.
//
// dataDir is the path to the base directory containing external data link files
// (typically "/usr/local/share/tast/data" on DUT). artifactURL is the URL of
// Google Cloud Storage directory, ending with a slash, containing build
// artifacts for the current Chrome OS image.
func NewManager(ctx context.Context, dataDir, artifactsURL string) (*Manager, error) {
	var all []string
	if err := filepath.Walk(dataDir, func(linkPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !strings.HasSuffix(linkPath, testing.ExternalLinkSuffix) {
			return nil
		}
		destPath := strings.TrimSuffix(linkPath, testing.ExternalLinkSuffix)
		all = append(all, destPath)
		return nil
	}); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("Failed to walk data directory: %v", err)
	}
	sort.Strings(all)

	return &Manager{
		dataDir:      dataDir,
		artifactsURL: artifactsURL,
		all:          all,
		inuse:        make(map[string]int),
	}, nil
}

// Purgeable returns a list of external data file paths not needed by the
// currently running entities. They can be deleted if the disk space is low.
func (m *Manager) Purgeable() []string {
	var res []string
	for _, p := range m.all {
		if m.inuse[p] > 0 {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			res = append(res, p)
		}
	}
	return res
}

// PrepareDownloads computes a list of external data files that need to be
// downloaded for entities.
//
// PrepareDownloads also removes stale files so they are never used even if we
// fail to download them later. When it encounters errors, *.external-error
// files are saved so that they can be read and reported by bundles later.
//
// PrepareDownloads returns a list of download job specifications that can be
// passed to RunDownloads to perform actual downloads.
//
// release must be called after entities finish.
func (m *Manager) PrepareDownloads(ctx context.Context, entities []*protocol.Entity) (jobs []*DownloadJob, release func()) {
	urlToJob := make(map[string]*DownloadJob)
	hasErr := false

	var releaseFunc []func()

	// Process tests.
	for _, t := range entities {
		for _, name := range t.GetDependencies().GetDataFiles() {
			destPath := filepath.Join(m.dataDir, testing.RelativeDataDir(t.Package), name)
			linkPath := destPath + testing.ExternalLinkSuffix
			errorPath := destPath + testing.ExternalErrorSuffix

			reportErr := func(format string, args ...interface{}) {
				msg := fmt.Sprintf("failed to prepare downloading %s: %s", name, fmt.Sprintf(format, args...))
				logging.Info(ctx, strings.ToUpper(msg[:1])+msg[1:])
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

			link, err := loadLink(linkPath, m.artifactsURL)
			if err != nil {
				reportErr("failed to load %s: %v", linkPath, err)
				continue
			}

			// This file is not purgeable.
			m.inuse[destPath]++
			releaseFunc = append(releaseFunc, func() {
				m.inuse[destPath]--
			})

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
			job := urlToJob[link.ComputedURL]
			if job == nil {
				job = &DownloadJob{link, nil}
				urlToJob[link.ComputedURL] = job
			} else if !reflect.DeepEqual(job.link, link) {
				reportErr("conflicting external data link found at %s: got %+v, want %+v", filepath.Join(testing.RelativeDataDir(t.Package), name), link, job.link)
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

	for _, j := range urlToJob {
		if len(j.dests) > 0 {
			jobs = append(jobs, j)
		}
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].link.ComputedURL < jobs[j].link.ComputedURL
	})

	logging.Infof(ctx, "Found %d external linked data file(s), need to download %d", len(urlToJob), len(jobs))
	if hasErr {
		logging.Info(ctx, "Encountered some errors on scanning external data link files, but continuing anyway; corresponding tests will fail")
	}
	return jobs, func() {
		for _, f := range releaseFunc {
			f()
		}
	}
}

// loadLink loads a JSON file of LinkData.
func loadLink(path, artifactsURL string) (*link, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var d LinkData
	if err := json.NewDecoder(f).Decode(&d); err != nil {
		return nil, err
	}

	l, err := newLink(&d, artifactsURL)
	if err != nil {
		return nil, err
	}
	return l, nil
}

// RunDownloads downloads required external data files in parallel.
//
// dataDir is the path to the base directory containing external data link files
// (typically "/usr/local/share/tast/data" on DUT). jobs are typically obtained
// by calling PrepareDownloads.
//
// This function does not return errors; instead it tries to download files as
// far as possible and logs encountered errors with ctx so that a single
// download error does not cause all tests to fail.
func RunDownloads(ctx context.Context, dataDir string, jobs []*DownloadJob, cl devserver.Client) {
	jobCh := make(chan *DownloadJob, len(jobs))
	for _, job := range jobs {
		jobCh <- job
	}
	close(jobCh)

	const parallelism = 4
	resCh := make(chan *downloadResult, len(jobs))
	for i := 0; i < parallelism; i++ {
		go func() {
			for job := range jobCh {
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
				msg := fmt.Sprintf("failed to download %s: %v", res.job.link.ComputedURL, res.err)
				logging.Info(ctx, strings.ToUpper(msg[:1])+msg[1:])
				for _, dest := range res.job.dests {
					ioutil.WriteFile(dest+testing.ExternalErrorSuffix, []byte(msg), 0666)
				}
				hasErr = true
			} else {
				mbs := float64(res.size) / res.duration.Seconds() / 1024 / 1024
				logging.Infof(ctx, "Finished downloading %s (%d bytes, %v, %.1fMB/s)",
					res.job.link.ComputedURL, res.size, res.duration.Round(time.Millisecond), mbs)
			}
			finished++
		case <-time.After(30 * time.Second):
			// Without this keep-alive message, the tast command may think that the SSH connection was lost.
			// TODO(nya): Remove this keep-alive logic after 20190701.
			logging.Info(ctx, "Still downloading...")
		}
	}

	if hasErr {
		logging.Info(ctx, "Failed to download some external data files, but continuing anyway; corresponding tests will fail")
	}
}

// runDownload downloads an external data file.
func runDownload(ctx context.Context, dataDir string, job *DownloadJob, cl devserver.Client) (size int64, retErr error) {
	// Create the temporary file under dataDir to make use of hard links.
	f, err := ioutil.TempFile(dataDir, ".external-download.")
	if err != nil {
		return 0, err
	}
	defer os.Remove(f.Name())
	defer func() {
		if err := f.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}()

	var mode os.FileMode = 0644
	if job.link.Data.Executable {
		mode = 0755
	}
	if err := f.Chmod(mode); err != nil {
		return 0, err
	}

	r, err := cl.Open(ctx, job.link.ComputedURL)
	if err != nil {
		return 0, err
	}
	defer r.Close()

	size, err = io.Copy(f, r)
	if err != nil {
		return size, err
	}

	if err := verify(f, job.link); err != nil {
		return size, err
	}

	for _, dest := range job.dests {
		if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
			return size, err
		}
		if err := os.Link(f.Name(), dest); err != nil {
			return size, err
		}
	}
	return size, nil
}

// verify checks the integrity of an external data file.
func verify(f *os.File, link *link) error {
	if link.Data.Type == TypeArtifact {
		// For artifacts, we do not verify files.
		return nil
	}

	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if fi.Size() != link.Data.Size {
		return fmt.Errorf("file size mismatch; got %d bytes, want %d bytes", fi.Size(), link.Data.Size)
	}

	if _, err := f.Seek(0, 0); err != nil {
		return err
	}

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return fmt.Errorf("failed to compute hash: %v", err)
	}
	hash := hex.EncodeToString(hasher.Sum(nil))
	if hash != link.Data.SHA256Sum {
		return fmt.Errorf("hash mismatch; got %s, want %s", hash, link.Data.SHA256Sum)
	}
	return nil
}
