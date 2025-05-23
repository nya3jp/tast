// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package devserver provides in-process devserver implementations.
package devserver

import (
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Ephemeral is a minimal devserver implementation using local credentials.
//
// An ephemeral devserver usually uses SSH reverse port forwarding to accept
// requests from local_test_runner on the DUT, and proxies requests to other
// servers such as Google Cloud Storage with proper credentials installed on the
// host. This allows unprivileged DUTs to access ACL'ed resources, such as
// private external data files on Google Cloud Storage.
type Ephemeral struct {
	server         *http.Server
	cacheDir       string
	allowedBuckets map[string]struct{}
}

// NewEphemeral starts a new ephemeral devserver listening on lis.
// Ownership of lis is taken, so the caller must not call lis.Close.
// A directory is created at cacheDir if it does not exist.
func NewEphemeral(lis net.Listener, cacheDir string, extraAllowedBuckets []string) (*Ephemeral, error) {
	defer func() {
		if lis != nil {
			lis.Close()
		}
	}()

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, err
	}

	allowedBuckets := defaultAllowedBuckets()
	for _, b := range extraAllowedBuckets {
		allowedBuckets[b] = struct{}{}
	}

	mux := http.NewServeMux()
	s := &Ephemeral{
		server:         &http.Server{Handler: mux},
		cacheDir:       cacheDir,
		allowedBuckets: allowedBuckets,
	}

	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/check_health", s.handleCheckHealth)
	mux.HandleFunc("/is_staged", s.handleIsStaged)
	mux.HandleFunc("/stage", s.handleStage)
	mux.HandleFunc("/extract/", s.handleExtract)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(cacheDir))))

	go s.server.Serve(lis)
	lis = nil // Server.Serve closes lis

	return s, nil
}

// Close shuts down the ephemeral devserver and releases associated resources.
func (e *Ephemeral) Close() error {
	return e.server.Close()
}

// handleIndex serves any requests not matched to any other routes.
func (e *Ephemeral) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	io.WriteString(w, "This is an ephemeral devserver provided by Tast.")
}

// handleCheckHealth serves requests to check the server health.
func (e *Ephemeral) handleCheckHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, "{}")
}

// handleIsStaged serves requests to check if a file is staged.
func (e *Ephemeral) handleIsStaged(w http.ResponseWriter, r *http.Request) {
	if err := func() error {
		q := r.URL.Query()
		// We only allow single file specified in &files=.
		gsURL := strings.TrimRight(q.Get("archive_url"), "/") + "/" + q.Get("files")

		relPath, err := e.validateGSURL(gsURL)
		if err != nil {
			return err
		}
		savePath := filepath.Join(e.cacheDir, relPath)

		if _, err := os.Stat(savePath); err == nil {
			io.WriteString(w, "True")
		} else if os.IsNotExist(err) {
			io.WriteString(w, "False")
		} else {
			return err
		}
		return nil
	}(); err != nil {
		writeError(w, err)
		return
	}
}

// handleStage serves requests to stage (i.e. download and cache) a file.
// In a real cache server, /stage is actually a no-op.
func (e *Ephemeral) handleStage(w http.ResponseWriter, r *http.Request) {
	if err := func() error {
		q := r.URL.Query()
		// We only allow single file specified in &files=.
		gsURL := strings.TrimRight(q.Get("archive_url"), "/") + "/" + q.Get("files")

		relPath, err := e.validateGSURL(gsURL)
		if err != nil {
			return err
		}
		savePath := filepath.Join(e.cacheDir, relPath)

		if _, err := os.Stat(savePath); err == nil {
			// Already staged.
			return nil
		}

		if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
			return err
		}

		tf, err := os.CreateTemp(e.cacheDir, ".download.")
		if err != nil {
			return err
		}
		tf.Close()
		defer os.Remove(tf.Name())

		// Use gsutil command to download a file to use the locally installed credentials.
		cmd := exec.Command("gsutil", "-m", "cp", gsURL, tf.Name())
		out, err := cmd.CombinedOutput()
		if err != nil {
			if strings.Contains(string(out), "No URLs matched") {
				w.Header().Add("Content-Type", "text/html")
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprintf(w, "<pre>\n%s\n</pre>", html.EscapeString(fmt.Sprintf("file not found: %s", gsURL)))
				return nil
			}
			return fmt.Errorf("%s failed: %v %s", strings.Join(cmd.Args, " "), err, string(out))
		}

		return os.Link(tf.Name(), savePath)
	}(); err != nil {
		writeError(w, err)
		return
	}

	io.WriteString(w, "Success")
}

// handleExtract handles the /extract/ url path.  Extracts a single file from a compressed tar archive.
// For compatibility with the devservers in the lab, extract all the files from the tar archive into the cache dir.
// Lab devservers are slow when requesting the first file, and then very fast for the subsequent files, so it must work that way also.
func (e *Ephemeral) handleExtract(w http.ResponseWriter, r *http.Request) {
	if err := func() error {
		// /extract/GS_BUCKET/GS_PATH/ARCHIVE.tar.xz?file=FILENAME
		gsURL := "gs:/" + strings.TrimPrefix(r.URL.Path, "/extract")
		extractFile := r.URL.Query().Get("file")

		relPath, err := e.validateGSURL(gsURL)
		if err != nil {
			return err
		}
		archivePath := filepath.Join(e.cacheDir, relPath)
		archiveFiles := fmt.Sprintf("%s_files", archivePath)
		extractedFile := filepath.Join(archiveFiles, extractFile)
		relPath, err = filepath.Rel(e.cacheDir, extractedFile)
		if err != nil {
			return err
		}
		// If the file exists, redirect to it.
		if _, err := os.Stat(extractedFile); err == nil {
			http.Redirect(w, r, fmt.Sprintf("/static/%s", relPath), http.StatusTemporaryRedirect)
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		// If the extracted dir exists, then the file isn't in the archive.
		if _, err := os.Stat(archiveFiles); err == nil {
			w.Header().Add("Content-Type", "text/html")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, "<pre>\n%s\n</pre>", html.EscapeString(fmt.Sprintf("file %q not found in %q", extractFile, archiveFiles)))
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}

		// Untar archive to a temp dir
		archiveFilesTemp, err := os.MkdirTemp(e.cacheDir, ".untar.")
		if err != nil {
			return err
		}
		defer os.RemoveAll(archiveFilesTemp)
		cmd := exec.Command("tar", "-C", archiveFilesTemp, "-xaf", archivePath)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s failed: %v %s", strings.Join(cmd.Args, " "), err, string(out))
		}

		// tar successful, rename temp dir
		if err := os.Rename(archiveFilesTemp, archiveFiles); err != nil {
			return err
		}

		// Redirect to the requested file
		http.Redirect(w, r, fmt.Sprintf("/static/%s", relPath), http.StatusTemporaryRedirect)
		return nil
	}(); err != nil {
		writeError(w, err)
		return
	}
}

// writeError responds to an HTTP request by 500 Internal Server Error and
// the error message in HTML.
func writeError(w http.ResponseWriter, err error) {
	w.Header().Add("Content-Type", "text/html")
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "<pre>\n%s\n</pre>", html.EscapeString(err.Error()))
}

// defaultAllowedBuckets returns a set of Google Cloud Storage buckets the ephemeral
// devserver is allowed to access by default.
func defaultAllowedBuckets() map[string]struct{} {
	return map[string]struct{}{
		"chromeos-image-archive":              {},
		"chromeos-moblab-intel":               {},
		"chromeos-releases":                   {},
		"chromeos-test-assets-partner-shared": {},
		"chromeos-test-assets-private":        {},
		"chromiumos-test-assets-public":       {},
		"cros-containers-staging":             {},
		"crosvideo":                           {},
		"firmware-image-archive":              {},
		"perfetto":                            {},
		"pstash_apks":                         {},
		"refvm-images":                        {},
		"sw-perf-meet-experiments":            {},
		"termina-component-testing":           {},
	}
}

// validateGSURL checks if the given Google Cloud Storage URL is a valid and
// allowed, and returns the path in the URL (without a leading slash).
func (e *Ephemeral) validateGSURL(gsURL string) (path string, err error) {
	p, err := url.Parse(gsURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL %q: %v", gsURL, err)
	}
	if p.Scheme != "gs" {
		return "", fmt.Errorf("%q is not a gs:// URL", gsURL)
	}
	if _, ok := e.allowedBuckets[p.Host]; !ok {
		return "", fmt.Errorf("%q doesn't use an allowed bucket (%s)", gsURL, p.Host)
	}
	if filepath.Clean(p.Path) != p.Path {
		return "", fmt.Errorf("%q isn't a clean URL", gsURL)
	}
	return strings.TrimLeft(p.Path, "/"), nil
}
