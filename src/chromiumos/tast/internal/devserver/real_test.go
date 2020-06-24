// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver_test

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"chromiumos/tast/internal/devserver"
)

const (
	fakeFileURL  = "gs://bucket/path/to/some%20file%2521"
	fakeFileData = "some_data"

	notFoundURL = "gs://bucket/path/to/not_found"
)

// fakeServerFiles defines fake files served from fakeServer.
var fakeServerFiles = map[string][]byte{
	fakeFileURL: []byte("some_data"),
}

type stagedEntry struct {
	data   []byte
	bucket string
}

type fakeServer struct {
	*httptest.Server

	up     bool
	staged map[string]*stagedEntry
	// stageFailCount makes stage request fail for this time.
	stageFailCount int
	dlCounter      map[string]int
}

func newFakeServer(up bool) *fakeServer {
	mux := http.NewServeMux()
	s := &fakeServer{
		Server:    httptest.NewServer(mux),
		up:        up,
		staged:    make(map[string]*stagedEntry),
		dlCounter: make(map[string]int),
	}
	mux.Handle("/check_health", http.HandlerFunc(s.handleCheckHealth))
	mux.Handle("/is_staged", http.HandlerFunc(s.handleIsStaged))
	mux.Handle("/stage", http.HandlerFunc(s.handleStage))
	mux.Handle("/static/", http.HandlerFunc(s.handleStatic))
	return s
}

func (s *fakeServer) close() {
	s.Server.Close()
}

func (s *fakeServer) handleCheckHealth(w http.ResponseWriter, r *http.Request) {
	if !s.up {
		respondError(w, errors.New("down"))
	}
}

func (s *fakeServer) handleIsStaged(w http.ResponseWriter, r *http.Request) {
	if err := func() error {
		q := r.URL.Query()
		gsURL := q.Get("archive_url") + "/" + url.PathEscape(q.Get("files"))
		_, stagePath, err := devserver.ParseGSURL(gsURL)
		if err != nil {
			return err
		}
		if _, ok := s.staged[stagePath]; ok {
			io.WriteString(w, "True")
		} else {
			io.WriteString(w, "False")
		}
		return nil
	}(); err != nil {
		respondError(w, err)
	}
}

func (s *fakeServer) handleStage(w http.ResponseWriter, r *http.Request) {
	if err := func() error {
		q := r.URL.Query()
		gsURL := q.Get("archive_url") + "/" + url.PathEscape(q.Get("files"))
		return s.stage(gsURL)
	}(); err != nil {
		respondError(w, err)
	}
}

func (s *fakeServer) handleStatic(w http.ResponseWriter, r *http.Request) {
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
	e, ok := s.staged[stagePath]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if bucket := r.URL.Query().Get("gs_bucket"); bucket != e.bucket {
		http.Error(w, fmt.Sprintf("Incorrect gs_bucket: got %q, want %q", bucket, e.bucket), http.StatusBadRequest)
		return
	}
	if r.Method != http.MethodHead {
		w.Write(e.data)
		s.dlCounter[stagePath]++
	}
}

func (s *fakeServer) stage(gsURL string) error {
	data, ok := fakeServerFiles[gsURL]
	if !ok {
		return errors.New("file not found")
	}
	bucket, stagePath, err := devserver.ParseGSURL(gsURL)
	if err != nil {
		return err
	}
	if s.stageFailCount > 0 {
		s.stageFailCount--
		return errors.New("failed to stage")
	}
	s.staged[stagePath] = &stagedEntry{
		data:   data,
		bucket: bucket,
	}
	return nil
}

func (s *fakeServer) unstage(gsURL string) error {
	_, stagePath, err := devserver.ParseGSURL(gsURL)
	if err != nil {
		return err
	}
	delete(s.staged, stagePath)
	return nil
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

// TestRealClientSimple tests the most simple case of successful download.
func TestRealClientSimple(t *testing.T) {
	s := newFakeServer(true)
	defer s.close()

	cl := devserver.NewRealClient(context.Background(), []string{s.URL}, nil)

	r, err := cl.Open(context.Background(), fakeFileURL)
	if err != nil {
		t.Error("Open failed: ", err)
	} else {
		defer r.Close()
		if data, err := ioutil.ReadAll(r); err != nil {
			t.Error("ReadAll failed: ", err)
		} else if string(data) != fakeFileData {
			t.Errorf("Open returned %q; want %q", string(data), fakeFileData)
		}
	}

	if r, err := cl.Open(context.Background(), "gs://bucket/path/to/wrong_file"); err == nil {
		r.Close()
		t.Error("Open unexpectedly succeeded")
	}
}

// TestRealClientNotFound tests when the file to be staged is not found.
func TestRealClientNotFound(t *testing.T) {
	s := newFakeServer(true)
	defer s.close()

	cl := devserver.NewRealClient(context.Background(), []string{s.URL}, nil)

	if r, err := cl.Open(context.Background(), notFoundURL); err == nil {
		r.Close()
		t.Error("Open unexpectedly succeeded")
	} else if !os.IsNotExist(err) {
		t.Errorf("Open returned %q; want %q", err, os.ErrNotExist)
	}
}

// TestRealClientPreferStagedServer tests that already staged servers are preferred.
func TestRealClientPreferStagedServer(t *testing.T) {
	s1 := newFakeServer(true)
	defer s1.close()
	s2 := newFakeServer(true)
	defer s2.close()

	cl := devserver.NewRealClient(context.Background(), []string{s1.URL, s2.URL}, nil)

	err := s1.stage(fakeFileURL)
	if err != nil {
		t.Fatal(err)
	}

	_, stagePath, _ := devserver.ParseGSURL(fakeFileURL)

	for i := 1; i <= 10; i++ {
		r, err := cl.Open(context.Background(), fakeFileURL)
		if err != nil {
			t.Fatal(err)
		}
		r.Close()
		c1 := s1.dlCounter[stagePath]
		c2 := s2.dlCounter[stagePath]
		if c1 != i || c2 != 0 {
			t.Fatalf("After %d request(s), dlCounter = (%d, %d); want (%d, %d)", i, c1, c2, i, 0)
		}
	}
}

// TestRealClientRetryStage tests that failed stage request is retried.
func TestRealClientRetryStage(t *testing.T) {
	s := newFakeServer(true)
	defer s.close()
	s.stageFailCount = 1

	o := &devserver.RealClientOptions{StageRetryWaits: []time.Duration{time.Duration(1 * time.Millisecond)}}
	cl := devserver.NewRealClient(context.Background(), []string{s.URL}, o)

	r, err := cl.Open(context.Background(), fakeFileURL)
	if err != nil {
		t.Error("Open failed despite retries: ", err)
	}
	r.Close()
}

// TestRealClientRetryStageFail tests too many failures causes the download to fail.
func TestRealClientRetryStageFail(t *testing.T) {
	s := newFakeServer(true)
	defer s.close()
	s.stageFailCount = 2

	o := &devserver.RealClientOptions{StageRetryWaits: []time.Duration{time.Duration(1 * time.Millisecond)}}
	cl := devserver.NewRealClient(context.Background(), []string{s.URL}, o)

	if r, err := cl.Open(context.Background(), fakeFileURL); err == nil {
		r.Close()
		t.Error("Open succeeded despite too many failures")
	}
}

// TestRealClientStableServerSelection tests that the server selection is stable.
func TestRealClientStableServerSelection(t *testing.T) {
	s1 := newFakeServer(true)
	defer s1.close()
	s2 := newFakeServer(true)
	defer s2.close()

	cl := devserver.NewRealClient(context.Background(), []string{s1.URL, s2.URL}, nil)

	// Download the file once and make sure s1 is always selected.
	r, err := cl.Open(context.Background(), fakeFileURL)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	_, stagePath, _ := devserver.ParseGSURL(fakeFileURL)
	if s2.dlCounter[stagePath] > 0 {
		s1, s2 = s2, s1
	}

	for i := 2; i <= 10; i++ {
		r, err := cl.Open(context.Background(), fakeFileURL)
		if err != nil {
			t.Fatal(err)
		}
		r.Close()
		c1 := s1.dlCounter[stagePath]
		c2 := s2.dlCounter[stagePath]
		if c1 != i || c2 != 0 {
			t.Fatalf("After %d request(s), dlCounter = (%d, %d); want (%d, %d)", i, c1, c2, i, 0)
		}
	}
}

// TestRealClientSomeUpServers tests that down servers are not selected.
func TestRealClientSomeUpServers(t *testing.T) {
	up1 := newFakeServer(true)
	defer up1.close()
	up2 := newFakeServer(true)
	defer up2.close()
	down := newFakeServer(false)
	defer down.close()

	cl := devserver.NewRealClient(context.Background(), []string{up1.URL, down.URL, up2.URL}, nil)

	actualUpURLs := cl.UpServerURLs()
	expectedUpURLs := []string{up1.URL, up2.URL}
	sort.Strings(actualUpURLs)
	sort.Strings(expectedUpURLs)
	if !reflect.DeepEqual(actualUpURLs, expectedUpURLs) {
		t.Errorf("UpServerURLs = %v; want %v", actualUpURLs, expectedUpURLs)
	}
}

// TestRealClientSomeUpServers tests the scenario where no servers are up.
func TestRealClientNoUpServer(t *testing.T) {
	down1 := newFakeServer(false)
	defer down1.close()
	down2 := newFakeServer(false)
	defer down2.close()

	cl := devserver.NewRealClient(context.Background(), []string{down1.URL, down2.URL}, nil)

	if upURLs := cl.UpServerURLs(); len(upURLs) > 0 {
		t.Errorf("UpServerURLs = %v; want nil", upURLs)
	}

	if r, err := cl.Open(context.Background(), fakeFileURL); err == nil {
		r.Close()
		t.Fatal("Open unexpectedly succeeded")
	}
}
