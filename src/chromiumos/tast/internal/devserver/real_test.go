// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver_test

import (
	"context"
	"errors"
	"io/ioutil"
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

	r, err := cl.Open(context.Background(), fakeFileURL)
	if err != nil {
		t.Error("Open failed despite retries: ", err)
	}
	r.Close()

	if calls != 2 {
		t.Errorf("stageHook called %d time(s); want 2 times", calls)
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

	if r, err := cl.Open(context.Background(), fakeFileURL); err == nil {
		r.Close()
		t.Error("Open succeeded despite too many failures")
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
