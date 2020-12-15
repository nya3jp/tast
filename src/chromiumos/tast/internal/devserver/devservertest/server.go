// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package devservertest provides a fake implementation of devservers.
package devservertest

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"time"

	"chromiumos/tast/internal/devserver"
)

// Server is a fake devserver implementation.
type Server struct {
	*httptest.Server
	down               bool
	stageHook          func(gsURL string) error
	abortDownloadAfter int

	mu    sync.Mutex
	files map[string]*File
}

// File represents a file served by a Server. A set of files served by a Server
// can be specified by the Files option.
type File struct {
	// URL is a gs:// URL of a file.
	URL string
	// Data is the content of a file.
	Data []byte
	// Staged indicates if the file has been staged or not.
	Staged bool
}

// Copy creates a deep copy of a File.
func (f *File) Copy() *File {
	return &File{
		URL:    f.URL,
		Data:   append([]byte(nil), f.Data...),
		Staged: f.Staged,
	}
}

type options struct {
	down               bool
	files              []*File
	stageHook          func(gsURL string) error
	abortDownloadAfter int
}

// Option is an option accepted by NewServer to configure Server initialization.
type Option func(o *options)

// Down returns an option to mark a Server down. Such a server responds negatively
// to health queries.
func Down() Option {
	return func(o *options) {
		o.down = true
	}
}

// Files returns an option to specify a set of files served by a Server.
func Files(files []*File) Option {
	return func(o *options) {
		o.files = files
	}
}

// StageHook returns an option to specify a hook function called before staging a file.
func StageHook(f func(gsURL string) error) Option {
	return func(o *options) {
		o.stageHook = f
	}
}

// AbortDownloadAfter returns an option to make download requests fail after specified bytes.
func AbortDownloadAfter(bytes int) Option {
	return func(o *options) {
		o.abortDownloadAfter = bytes
	}
}

// NewServer starts a fake devserver using specified options.
func NewServer(opts ...Option) (*Server, error) {
	mux := http.NewServeMux()
	o := &options{
		stageHook:          func(gsURL string) error { return nil },
		abortDownloadAfter: -1,
	}
	for _, opt := range opts {
		opt(o)
	}

	files := make(map[string]*File)
	for _, f := range o.files {
		_, stagePath, err := devserver.ParseGSURL(f.URL)
		if err != nil {
			return nil, err
		}
		if _, ok := files[stagePath]; ok {
			return nil, fmt.Errorf("duplicated file at %s", stagePath)
		}
		files[stagePath] = f.Copy()
	}

	s := &Server{
		Server:             httptest.NewServer(mux),
		down:               o.down,
		stageHook:          o.stageHook,
		abortDownloadAfter: o.abortDownloadAfter,
		files:              files,
	}

	mux.Handle("/check_health", http.HandlerFunc(s.handleCheckHealth))
	mux.Handle("/is_staged", http.HandlerFunc(s.handleIsStaged))
	mux.Handle("/stage", http.HandlerFunc(s.handleStage))
	mux.Handle("/static/", http.HandlerFunc(s.handleStatic))
	return s, nil
}

// Close stops the server and releases its associated resources.
func (s *Server) Close() {
	s.Server.Close()
}

func (s *Server) handleCheckHealth(w http.ResponseWriter, r *http.Request) {
	if s.down {
		respondError(w, errors.New("down"))
	}
}

func (s *Server) handleIsStaged(w http.ResponseWriter, r *http.Request) {
	if err := func() error {
		q := r.URL.Query()
		gsURL := q.Get("archive_url") + "/" + url.PathEscape(q.Get("files"))
		_, stagePath, err := devserver.ParseGSURL(gsURL)
		if err != nil {
			return err
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		if f, ok := s.files[stagePath]; ok && f.Staged {
			io.WriteString(w, "True")
		} else {
			io.WriteString(w, "False")
		}
		return nil
	}(); err != nil {
		respondError(w, err)
	}
}

func (s *Server) handleStage(w http.ResponseWriter, r *http.Request) {
	if err := func() error {
		q := r.URL.Query()
		gsURL := q.Get("archive_url") + "/" + url.PathEscape(q.Get("files"))

		if err := s.stageHook(gsURL); err != nil {
			return err
		}

		_, stagePath, err := devserver.ParseGSURL(gsURL)
		if err != nil {
			return err
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		f, ok := s.files[stagePath]
		if !ok {
			return errors.New("file not found")
		}

		f.Staged = true
		return nil
	}(); err != nil {
		respondError(w, err)
	}
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Negotiate") != "vlist" {
		http.Error(w, "Negotiate: vlist is required", http.StatusBadRequest)
		return
	}

	// Python devserver distinguishes "/" and "%2F". We follow the way here.
	path, err := pathUnescape(r.URL.EscapedPath())
	if err != nil {
		respondError(w, err)
		return
	}
	stagePath := strings.TrimPrefix(path, "/static/")

	s.mu.Lock()
	defer s.mu.Unlock()

	f, ok := s.files[stagePath]
	if !ok || !f.Staged {
		http.NotFound(w, r)
		return
	}

	bucket, _, err := devserver.ParseGSURL(f.URL)
	if err != nil {
		respondError(w, err)
		return
	}
	if b := r.URL.Query().Get("gs_bucket"); b != bucket {
		http.Error(w, fmt.Sprintf("Incorrect gs_bucket: got %q, wantStaged %q", b, bucket), http.StatusBadRequest)
		return
	}

	if s.abortDownloadAfter >= 0 {
		w = newCappedResponseWriter(w, s.abortDownloadAfter)
	}
	http.ServeContent(w, r, path, time.Unix(0, 0), bytes.NewReader(f.Data))
}

// cappedResponseWriter wraps http.ResponseWriter with response size limit.
type cappedResponseWriter struct {
	http.ResponseWriter
	remaining int
}

func newCappedResponseWriter(w http.ResponseWriter, cap int) *cappedResponseWriter {
	return &cappedResponseWriter{ResponseWriter: w, remaining: cap}
}

func (w *cappedResponseWriter) Write(p []byte) (int, error) {
	if len(p) > w.remaining {
		p = p[:w.remaining]
	}
	w.remaining -= len(p)
	return w.ResponseWriter.Write(p)
}

func respondError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "<pre>\n%s\n</pre>", html.EscapeString(err.Error()))
}

// pathUnescape unescapes the path part of a URL. It fails if the path contains %2F.
func pathUnescape(escaped string) (string, error) {
	if escaped == "" {
		return "", nil
	}

	comps := strings.Split(escaped, "/")
	for i, c := range comps {
		uc, err := url.PathUnescape(c)
		if err != nil {
			return "", err
		} else if strings.Contains(uc, "/") {
			return "", errors.New("invalid path encoding")
		}
		comps[i] = uc
	}
	return strings.Join(comps, "/"), nil
}
