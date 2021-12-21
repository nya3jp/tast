// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver_test

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	"chromiumos/tast/internal/devserver"
	"chromiumos/tast/internal/devserver/devservertest"
)

const (
	fakeFileURL  = "gs://bucket/path/to/some%20file%2521"
	fakeFileData = "some_data"

	notFoundURL = "gs://bucket/path/to/not_found"
)

func newFakeServer(t *testing.T, opts ...devservertest.Option) *devservertest.Server {
	t.Helper()
	files := []*devservertest.File{{URL: fakeFileURL, Data: []byte(fakeFileData)}}
	opts = append([]devservertest.Option{devservertest.Files(files)}, opts...)
	s, err := devservertest.NewServer(opts...)
	if err != nil {
		t.Fatal("NewServer: ", err)
	}
	return s
}

// TestRealClientSimple tests the most simple case of successful download.
func TestRealClientSimple(t *testing.T) {
	s := newFakeServer(t)
	defer s.Close()

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

// TestRealClientSimpleStage tests the most simple case of successful download.
func TestRealClientSimpleStage(t *testing.T) {
	s := newFakeServer(t)
	defer s.Close()

	cl := devserver.NewRealClient(context.Background(), []string{s.URL}, nil)

	fileURL, err := cl.Stage(context.Background(), fakeFileURL)
	if err != nil {
		t.Error("Stage failed: ", err)
	} else {
		req, err := http.NewRequest(http.MethodGet, fileURL.String(), nil)
		if err != nil {
			t.Errorf("NewRequest %q failed: %v", fileURL, err)
		}
		req.Header.Set("Negotiate", "vlist")
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			t.Errorf("Get %q failed: %v", fileURL, err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("Get %q failed: %v", fileURL, resp)
		}
		defer resp.Body.Close()
		if data, err := ioutil.ReadAll(resp.Body); err != nil {
			t.Error("ReadAll failed: ", err)
		} else if string(data) != fakeFileData {
			t.Errorf("ReadAll returned %q; want %q", string(data), fakeFileData)
		}
	}

	if _, err := cl.Stage(context.Background(), "gs://bucket/path/to/wrong_file"); err == nil {
		t.Error("Stage unexpectedly succeeded")
	}
}

// TestRealClientNotFound tests when the file to be staged is not found.
func TestRealClientNotFound(t *testing.T) {
	s := newFakeServer(t)
	defer s.Close()

	cl := devserver.NewRealClient(context.Background(), []string{s.URL}, nil)

	if r, err := cl.Open(context.Background(), notFoundURL); err == nil {
		r.Close()
		t.Error("Open unexpectedly succeeded")
	} else if !os.IsNotExist(err) {
		t.Errorf("Open returned %q; want %q", err, os.ErrNotExist)
	}
}

// TestRealClientNotFoundStage tests when the file to be staged is not found.
func TestRealClientNotFoundStage(t *testing.T) {
	s := newFakeServer(t)
	defer s.Close()

	cl := devserver.NewRealClient(context.Background(), []string{s.URL}, nil)

	if fileURL, err := cl.Stage(context.Background(), notFoundURL); err == nil {
		t.Error("Stage unexpectedly succeeded: ", fileURL)
	} else if !os.IsNotExist(err) {
		t.Errorf("Stage returned %q; want %q", err, os.ErrNotExist)
	}
}

// TestRealClientPreferStagedServer tests that already staged servers are preferred.
func TestRealClientPreferStagedServer(t *testing.T) {
	stageHook := func(gsURL string) error {
		t.Error("Unexpected attempt to stage: ", gsURL)
		return errors.New("stage failure")
	}

	// First server doesn't have a staged file. Any attempt to stage a file will
	// result in a test failure.
	files1 := []*devservertest.File{{URL: fakeFileURL, Data: []byte(fakeFileData)}}
	s1, err := devservertest.NewServer(devservertest.Files(files1), devservertest.StageHook(stageHook))
	if err != nil {
		t.Fatal("NewServer: ", err)
	}
	defer s1.Close()

	// Second server has a staged file.
	files2 := []*devservertest.File{files1[0].Copy()}
	files2[0].Staged = true
	s2, err := devservertest.NewServer(devservertest.Files(files2), devservertest.StageHook(stageHook))
	if err != nil {
		t.Fatal("NewServer: ", err)
	}
	defer s2.Close()

	cl := devserver.NewRealClient(context.Background(), []string{s1.URL, s2.URL}, nil)

	for i := 1; i <= 10; i++ {
		r, err := cl.Open(context.Background(), fakeFileURL)
		if err != nil {
			t.Fatal(err)
		}
		r.Close()
	}
}

// TestRealClientPreferStagedServerStage tests that already staged servers are preferred.
func TestRealClientPreferStagedServerStage(t *testing.T) {
	stageHook := func(gsURL string) error {
		t.Error("Unexpected attempt to stage: ", gsURL)
		return errors.New("stage failure")
	}

	// First server doesn't have a staged file. Any attempt to stage a file will
	// result in a test failure.
	files1 := []*devservertest.File{{URL: fakeFileURL, Data: []byte(fakeFileData)}}
	s1, err := devservertest.NewServer(devservertest.Files(files1), devservertest.StageHook(stageHook))
	if err != nil {
		t.Fatal("NewServer: ", err)
	}
	defer s1.Close()

	// Second server has a staged file.
	files2 := []*devservertest.File{files1[0].Copy()}
	files2[0].Staged = true
	s2, err := devservertest.NewServer(devservertest.Files(files2), devservertest.StageHook(stageHook))
	if err != nil {
		t.Fatal("NewServer: ", err)
	}
	defer s2.Close()

	cl := devserver.NewRealClient(context.Background(), []string{s1.URL, s2.URL}, nil)

	for i := 1; i <= 10; i++ {
		_, err := cl.Stage(context.Background(), fakeFileURL)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestRealClientRetryStageOpen tests that failed stage request is retried.
func TestRealClientRetryStageOpen(t *testing.T) {
	calls := 0
	stageHook := func(gsURL string) error {
		calls++
		if calls <= 1 {
			return errors.New("stage failure")
		}
		return nil
	}
	s := newFakeServer(t, devservertest.StageHook(stageHook))
	defer s.Close()

	o := &devserver.RealClientOptions{StageRetryWaits: []time.Duration{1 * time.Millisecond}}
	cl := devserver.NewRealClient(context.Background(), []string{s.URL}, o)

	r, err := cl.Open(context.Background(), fakeFileURL)
	if err != nil {
		t.Error("Open failed despite retries: ", err)
	}
	r.Close()

	if calls != 2 {
		t.Errorf("stageHook called %d time(s); want 2 times", calls)
	}
}

// TestRealClientRetryStage tests that failed stage request is retried.
func TestRealClientRetryStage(t *testing.T) {
	calls := 0
	stageHook := func(gsURL string) error {
		calls++
		if calls <= 1 {
			return errors.New("stage failure")
		}
		return nil
	}
	s := newFakeServer(t, devservertest.StageHook(stageHook))
	defer s.Close()

	o := &devserver.RealClientOptions{StageRetryWaits: []time.Duration{1 * time.Millisecond}}
	cl := devserver.NewRealClient(context.Background(), []string{s.URL}, o)

	_, err := cl.Stage(context.Background(), fakeFileURL)
	if err != nil {
		t.Error("Stage failed despite retries: ", err)
	}

	if calls != 2 {
		t.Errorf("stageHook called %d time(s); want 2 times", calls)
	}
}

// TestRealClientRetryStageFailOpen tests too many failures causes the download to fail.
func TestRealClientRetryStageFailOpen(t *testing.T) {
	calls := 0
	stageHook := func(gsURL string) error {
		calls++
		if calls <= 2 {
			return errors.New("stage failure")
		}
		return nil
	}
	s := newFakeServer(t, devservertest.StageHook(stageHook))
	defer s.Close()

	o := &devserver.RealClientOptions{StageRetryWaits: []time.Duration{1 * time.Millisecond}}
	cl := devserver.NewRealClient(context.Background(), []string{s.URL}, o)

	if r, err := cl.Open(context.Background(), fakeFileURL); err == nil {
		r.Close()
		t.Error("Open succeeded despite too many failures")
	}
}

// TestRealClientRetryStageFail tests too many failures causes the download to fail.
func TestRealClientRetryStageFail(t *testing.T) {
	calls := 0
	stageHook := func(gsURL string) error {
		calls++
		if calls <= 2 {
			return errors.New("stage failure")
		}
		return nil
	}
	s := newFakeServer(t, devservertest.StageHook(stageHook))
	defer s.Close()

	o := &devserver.RealClientOptions{StageRetryWaits: []time.Duration{1 * time.Millisecond}}
	cl := devserver.NewRealClient(context.Background(), []string{s.URL}, o)

	if _, err := cl.Stage(context.Background(), fakeFileURL); err == nil {
		t.Error("Stage succeeded despite too many failures")
	}
}

// TestRealClientRetryDownload tests that interrupted download sessions are retried.
func TestRealClientRetryDownload(t *testing.T) {
	files := []*devservertest.File{{URL: fakeFileURL, Data: []byte(fakeFileData)}}
	// This fake server closes the connection after sending 3 bytes, so
	// we need to retry downloading to get the full content.
	s := newFakeServer(t, devservertest.Files(files), devservertest.AbortDownloadAfter(3))
	defer s.Close()

	cl := devserver.NewRealClient(context.Background(), []string{s.URL}, &devserver.RealClientOptions{})

	r, err := cl.Open(context.Background(), fakeFileURL)
	if err != nil {
		t.Error("Open failed despite retries: ", err)
	}
	defer r.Close()

	b, err := ioutil.ReadAll(r)
	if err != nil {
		t.Error("Error during downloading file: ", err)
	}

	data := string(b)
	if data != fakeFileData {
		t.Errorf("Content mismatch: got %q, want %q", data, fakeFileData)
	}
}

// TestRealClientRetryDownloadSuccessiveResumableErrors tests that download
// aborts after successive resumable failures.
func TestRealClientRetryDownloadSuccessiveResumableErrors(t *testing.T) {
	files := []*devservertest.File{{URL: fakeFileURL, Data: []byte(fakeFileData)}}
	// This fake server closes the connection before sending any data.
	s := newFakeServer(t, devservertest.Files(files), devservertest.AbortDownloadAfter(0))
	defer s.Close()

	cl := devserver.NewRealClient(context.Background(), []string{s.URL}, &devserver.RealClientOptions{})

	r, err := cl.Open(context.Background(), fakeFileURL)
	if err != nil {
		t.Error("Open failed despite retries: ", err)
	}
	defer r.Close()

	_, err = ioutil.ReadAll(r)
	if err == nil {
		t.Error("ReadAll succeeded unexpectedly")
	} else if err != io.ErrUnexpectedEOF {
		t.Errorf("ReadAll: %v; want %v", err, io.ErrUnexpectedEOF)
	}
}

// TestRealClientStableServerSelection tests that the server selection is stable.
func TestRealClientStableServerSelection(t *testing.T) {
	calls := 0
	stageHook := func(gsURL string) error {
		calls++
		return nil
	}
	s1 := newFakeServer(t, devservertest.StageHook(stageHook))
	defer s1.Close()
	s2 := newFakeServer(t, devservertest.StageHook(stageHook))
	defer s2.Close()

	cl := devserver.NewRealClient(context.Background(), []string{s1.URL, s2.URL}, nil)
	for i := 0; i < 10; i++ {
		r, err := cl.Open(context.Background(), fakeFileURL)
		if err != nil {
			t.Fatal(err)
		}
		r.Close()
	}
	if calls != 1 {
		t.Errorf("stageHook called %d time(s); want 1 time", calls)
	}
}

// TestRealClientStableServerSelectionStage tests that the server selection is stable.
func TestRealClientStableServerSelectionStage(t *testing.T) {
	calls := 0
	stageHook := func(gsURL string) error {
		calls++
		return nil
	}
	s1 := newFakeServer(t, devservertest.StageHook(stageHook))
	defer s1.Close()
	s2 := newFakeServer(t, devservertest.StageHook(stageHook))
	defer s2.Close()

	cl := devserver.NewRealClient(context.Background(), []string{s1.URL, s2.URL}, nil)
	for i := 0; i < 10; i++ {
		_, err := cl.Stage(context.Background(), fakeFileURL)
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Errorf("stageHook called %d time(s); want 1 time", calls)
	}
}

// TestRealClientSomeUpServers tests that down servers are not selected.
func TestRealClientSomeUpServers(t *testing.T) {
	up1 := newFakeServer(t)
	defer up1.Close()
	up2 := newFakeServer(t)
	defer up2.Close()
	down := newFakeServer(t, devservertest.Down())
	defer down.Close()

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
	down1 := newFakeServer(t, devservertest.Down())
	defer down1.Close()
	down2 := newFakeServer(t, devservertest.Down())
	defer down2.Close()

	cl := devserver.NewRealClient(context.Background(), []string{down1.URL, down2.URL}, nil)

	if upURLs := cl.UpServerURLs(); len(upURLs) > 0 {
		t.Errorf("UpServerURLs = %v; want nil", upURLs)
	}

	if r, err := cl.Open(context.Background(), fakeFileURL); err == nil {
		r.Close()
		t.Fatal("Open unexpectedly succeeded")
	}
}

// TestRealClientNoUpServerStage tests the scenario where no servers are up.
func TestRealClientNoUpServerStage(t *testing.T) {
	down1 := newFakeServer(t, devservertest.Down())
	defer down1.Close()
	down2 := newFakeServer(t, devservertest.Down())
	defer down2.Close()

	cl := devserver.NewRealClient(context.Background(), []string{down1.URL, down2.URL}, nil)

	if upURLs := cl.UpServerURLs(); len(upURLs) > 0 {
		t.Errorf("UpServerURLs = %v; want nil", upURLs)
	}

	if u, err := cl.Stage(context.Background(), fakeFileURL); err == nil {
		t.Fatal("Stage unexpectedly succeeded: ", u)
	}
}

// TestRealClientNoPath ensures that RealClient works for URLs having no path,
// i.e. files are directly under the top-level directory.
func TestRealClientNoPath(t *testing.T) {
	const url = "gs://bucket/file"

	files := []*devservertest.File{{URL: url}}
	s, err := devservertest.NewServer(devservertest.Files(files))
	if err != nil {
		t.Fatal("NewServer: ", err)
	}
	defer s.Close()

	cl := devserver.NewRealClient(context.Background(), []string{s.URL}, nil)

	r, err := cl.Open(context.Background(), url)
	if err != nil {
		t.Fatal("Open failed: ", err)
	}
	r.Close()
}

// TestRealClientNoPathStage ensures that RealClient works for URLs having no path,
// i.e. files are directly under the top-level directory.
func TestRealClientNoPathStage(t *testing.T) {
	const url = "gs://bucket/file"

	files := []*devservertest.File{{URL: url}}
	s, err := devservertest.NewServer(devservertest.Files(files))
	if err != nil {
		t.Fatal("NewServer: ", err)
	}
	defer s.Close()

	cl := devserver.NewRealClient(context.Background(), []string{s.URL}, nil)

	_, err = cl.Stage(context.Background(), url)
	if err != nil {
		t.Fatal("Stage failed: ", err)
	}
}
