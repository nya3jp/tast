// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"context"
	"fmt"
	"html"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ephemeralDevserverPort is the TCP port number the ephemeral devserver listens on.
// Real devservers listen on port 8082, so we use a similar but different port
// to avoid conflict.
const ephemeralDevserverPort = 28082

// ephemeralDevserver is a minimal devserver implementation using local credentials.
//
// An ephemeral devserver usually uses SSH reverse port forwarding to accept
// requests from local_test_runner on the DUT, and proxies requests to other
// servers such as Google Cloud Storage with proper credentials installed on the
// host. This allows unprivileged DUTs to access ACL'ed resources, such as
// private external data files on Google Cloud Storage.
type ephemeralDevserver struct {
	server   *http.Server
	cacheDir string
}

// newEphemeralDevserver starts a new ephemeral devserver listening on lis.
// Ownership of lis is taken, so the caller must not call lis.Close.
// A directory is created at cacheDir if it does not exist.
func newEphemeralDevserver(lis net.Listener, cacheDir string) (*ephemeralDevserver, error) {
	defer func() {
		if lis != nil {
			lis.Close()
		}
	}()

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	s := &ephemeralDevserver{
		server:   &http.Server{Handler: mux},
		cacheDir: cacheDir,
	}

	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/check_health", s.handleCheckHealth)
	mux.HandleFunc("/stage", s.handleStage)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(cacheDir))))

	go s.server.Serve(lis)
	lis = nil // Server.Serve closes lis

	return s, nil
}

// Close shuts down the ephemeral devserver and releases associated resources.
func (s *ephemeralDevserver) Close(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// handleIndex serves any requests not matched to any other routes.
func (s *ephemeralDevserver) handleIndex(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "This is an ephemeral devserver provided by Tast.")
}

// handleCheckHealth serves requests to check the server health.
func (s *ephemeralDevserver) handleCheckHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, "{}")
}

// handleStage serves requests to stage (i.e. download and cache) a file.
func (s *ephemeralDevserver) handleStage(w http.ResponseWriter, r *http.Request) {
	if err := func() error {
		q := r.URL.Query()
		gsURL := strings.TrimRight(q.Get("archive_url"), "/") + "/" + q.Get("files")

		relPath, err := parseGSURL(gsURL)
		if err != nil {
			return err
		}
		savePath := filepath.Join(s.cacheDir, relPath)

		if _, err := os.Stat(savePath); err == nil {
			// Already staged.
			return nil
		}

		if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
			return err
		}

		tf, err := ioutil.TempFile(s.cacheDir, ".download.")
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
				return fmt.Errorf("file not found: %s", gsURL)
			}
			return fmt.Errorf("%s failed: %v", strings.Join(cmd.Args, " "), err)
		}

		return os.Link(tf.Name(), savePath)
	}(); err != nil {
		writeError(w, err)
		return
	}

	io.WriteString(w, "Success")
}

// writeError responds to an HTTP request by 500 Internal Server Error and
// the error message in HTML.
func writeError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "<pre>\n%s\n</pre>", html.EscapeString(err.Error()))
}

// whitelistedBuckets is a set of Google Cloud Storage buckets the ephemeral
// devserver is allowed to access.
var whitelistedBuckets = map[string]struct{}{
	"chromeos-image-archive":        {},
	"chromeos-test-assets-private":  {},
	"chromiumos-test-assets-public": {},
}

// parseGSURL checks if the given Google Cloud Storage URL is a valid and
// whitelisted, and returns the path in the URL (without a leading slash).
func parseGSURL(gsURL string) (path string, err error) {
	p, err := url.Parse(gsURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL %q: %v", gsURL, err)
	}
	if p.Scheme != "gs" {
		return "", fmt.Errorf("%q is not a gs:// URL", gsURL)
	}
	if _, ok := whitelistedBuckets[p.Host]; !ok {
		return "", fmt.Errorf("%q doesn't use a whitelisted bucket", gsURL)
	}
	if filepath.Clean(p.Path) != p.Path {
		return "", fmt.Errorf("%q isn't a clean URL", gsURL)
	}
	return strings.TrimLeft(p.Path, "/"), nil
}
