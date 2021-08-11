// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devservertest_test

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/devserver/devservertest"
)

func TestCheckHealth(t *testing.T) {
	s, err := devservertest.NewServer()
	if err != nil {
		t.Fatal("NewServer: ", err)
	}
	defer s.Close()

	res, err := http.Get(s.URL + "/check_health")
	if err != nil {
		t.Fatal("http.Get: ", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		t.Errorf("StatusCode = %d; want 200", res.StatusCode)
	}
}

func TestCheckHealthDown(t *testing.T) {
	s, err := devservertest.NewServer(devservertest.Down())
	if err != nil {
		t.Fatal("NewServer: ", err)
	}
	defer s.Close()

	res, err := http.Get(s.URL + "/check_health")
	if err != nil {
		t.Fatal("http.Get: ", err)
	}
	defer res.Body.Close()

	if res.StatusCode == 200 {
		t.Errorf("StatusCode = %d; want non-200", res.StatusCode)
	}
}

func TestStage(t *testing.T) {
	const archiveURL = "gs://bucket/path/to"
	files := []*devservertest.File{
		{URL: archiveURL + "/file1.txt", Data: []byte("data1")},
		{URL: archiveURL + "/file2.txt", Data: []byte("data2")},
		{URL: archiveURL + "/file3.txt", Data: []byte("data3"), Staged: true},
	}
	s, err := devservertest.NewServer(devservertest.Files(files))
	if err != nil {
		t.Fatal("NewServer: ", err)
	}
	defer s.Close()

	// checkStaged queries /is_staged to check if a file is staged.
	checkStaged := func(fileName string) bool {
		t.Helper()

		params := make(url.Values)
		params.Add("archive_url", archiveURL)
		params.Add("files", fileName)
		u := s.URL + "/is_staged?" + params.Encode()

		res, err := http.Get(u)
		if err != nil {
			t.Fatal("http.Get: ", err)
		}
		defer res.Body.Close()

		if res.StatusCode != 200 {
			t.Fatalf("StatusCode = %d; want 200", res.StatusCode)
		}

		b, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatal("ReadAll: ", err)
		}

		switch string(b) {
		case "False":
			return false
		case "True":
			return true
		default:
			t.Fatalf("is_staged returned unexpected response %q", string(b))
			return false
		}
	}

	// readStaged queries /static to check if a file is staged.
	readStaged := func(fileName string) (data []byte, ok bool) {
		t.Helper()

		u := s.URL + "/static/path/to/" + fileName + "?gs_bucket=bucket"
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			t.Fatal("http.NewRequest: ", err)
		}
		req.Header.Add("Negotiate", "vlist")

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal("http.Do: ", err)
		}
		defer res.Body.Close()

		switch res.StatusCode {
		case 200:
			b, err := ioutil.ReadAll(res.Body)
			if err != nil {
				t.Fatal("ioutil.ReadAll: ", err)
			}
			return b, true
		case 404:
			return nil, false
		default:
			t.Fatalf("StatusCode = %d; want 200 or 404", res.StatusCode)
			return nil, false
		}
	}

	// verifyStaged calls checkStaged and readStaged, and checks if their results
	// match expectation.
	verifyStaged := func(fileName string, wantStaged bool, wantData []byte) {
		if staged := checkStaged(fileName); staged != wantStaged {
			t.Errorf("checkStaged(%q) = %t; want %t", fileName, staged, wantStaged)
		}
		data, staged := readStaged(fileName)
		if staged != wantStaged {
			t.Errorf("readStaged(%q) = %t; want %t", fileName, staged, wantStaged)
		} else if staged && !bytes.Equal(data, wantData) {
			t.Errorf("readStaged(%q) = %v; want %v", fileName, data, wantData)
		}
	}

	// stage queries /stage to stage a file.
	stage := func(fileName string) {
		t.Helper()

		params := make(url.Values)
		params.Add("archive_url", archiveURL)
		params.Add("files", fileName)
		u := s.URL + "/stage?" + params.Encode()

		res, err := http.Get(u)
		if err != nil {
			t.Fatal("http.Get: ", err)
		}
		defer res.Body.Close()

		if res.StatusCode != 200 {
			t.Fatalf("StatusCode = %d; want 200", res.StatusCode)
		}
	}

	// Initially only file3.txt is staged.
	for _, tc := range []struct {
		fileName   string
		wantStaged bool
		wantData   []byte
	}{
		{"file1.txt", false, nil},
		{"file2.txt", false, nil},
		{"file3.txt", true, []byte("data3")},
	} {
		verifyStaged(tc.fileName, tc.wantStaged, tc.wantData)
	}

	// Stage file2.txt and file3.txt. Staging is idempotent, so staging a file
	// multiple times should succeed.
	for _, fileName := range []string{"file2.txt", "file3.txt", "file2.txt"} {
		stage(fileName)
	}

	// Now file2.txt and file3.txt are staged.
	for _, tc := range []struct {
		fileName   string
		wantStaged bool
		wantData   []byte
	}{
		{"file1.txt", false, nil},
		{"file2.txt", true, []byte("data2")},
		{"file3.txt", true, []byte("data3")},
	} {
		verifyStaged(tc.fileName, tc.wantStaged, tc.wantData)
	}
}

func TestStageHook(t *testing.T) {
	calls := make(map[string]int)
	stageHook := func(gsURL string) error {
		calls[gsURL]++
		if gsURL == "gs://bucket/fail" {
			return errors.New("failed")
		}
		return nil
	}

	files := []*devservertest.File{{URL: "gs://bucket/pass"}, {URL: "gs://bucket/fail"}}
	s, err := devservertest.NewServer(devservertest.Files(files), devservertest.StageHook(stageHook))
	if err != nil {
		t.Fatal("NewServer: ", err)
	}
	defer s.Close()

	for _, tc := range []struct {
		url        string
		wantStatus int
	}{
		{s.URL + "/stage?archive_url=gs://bucket&files=pass", 200},
		{s.URL + "/stage?archive_url=gs://bucket&files=fail", 500},
		{s.URL + "/stage?archive_url=gs://bucket&files=miss", 500},
	} {
		res, err := http.Get(tc.url)
		if err != nil {
			t.Fatal("http.Get: ", err)
		}
		res.Body.Close()
		if res.StatusCode != tc.wantStatus {
			t.Errorf("StatusCode = %d; want %d", res.StatusCode, tc.wantStatus)
		}
	}

	wantCalls := map[string]int{
		"gs://bucket/pass": 1,
		"gs://bucket/fail": 1,
		"gs://bucket/miss": 1,
	}
	if diff := cmp.Diff(calls, wantCalls); diff != "" {
		t.Error("Calls mismatch (-got +want):\n", diff)
	}
}

func TestDownloadPartialContent(t *testing.T) {
	const data = "abcdefghijklmnopqrstuvwxyz"

	files := []*devservertest.File{{
		URL:    "gs://bucket/file",
		Data:   []byte(data),
		Staged: true,
	}}
	s, err := devservertest.NewServer(devservertest.Files(files))
	if err != nil {
		t.Fatal("NewServer: ", err)
	}
	defer s.Close()

	req, err := http.NewRequest(http.MethodGet, s.URL+"/static/file?gs_bucket=bucket", nil)
	if err != nil {
		t.Fatal("http.NewRequest: ", err)
	}
	req.Header.Add("Negotiate", "vlist")
	req.Header.Add("Range", "bytes=10-14")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal("http.Get: ", err)
	}
	defer res.Body.Close()

	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Error("ReadAll: ", err)
	}

	got := string(b)
	const want = "klmno"
	if got != want {
		t.Errorf("Content = %q; want %q", got, want)
	}
}

func TestAbortDownloadAfter(t *testing.T) {
	const (
		fullSize = 1000
		capSize  = 123
	)

	files := []*devservertest.File{{
		URL:    "gs://bucket/file",
		Data:   bytes.Repeat([]byte{'a'}, fullSize),
		Staged: true,
	}}
	s, err := devservertest.NewServer(devservertest.Files(files), devservertest.AbortDownloadAfter(capSize))
	if err != nil {
		t.Fatal("NewServer: ", err)
	}
	defer s.Close()

	req, err := http.NewRequest(http.MethodGet, s.URL+"/static/file?gs_bucket=bucket", nil)
	if err != nil {
		t.Fatal("http.NewRequest: ", err)
	}
	req.Header.Add("Negotiate", "vlist")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal("http.Get: ", err)
	}
	defer res.Body.Close()

	b, err := ioutil.ReadAll(res.Body)
	if err == nil {
		t.Error("ReadAll succeeded unexpectedly")
	} else if err != io.ErrUnexpectedEOF {
		t.Errorf("ReadAll: %v; want %v", err, io.ErrUnexpectedEOF)
	}

	got := string(b)
	want := strings.Repeat("a", capSize)
	if got != want {
		t.Errorf("Content = %q; want %q", got, want)
	}
}

func TestNegotiate(t *testing.T) {
	files := []*devservertest.File{{URL: "gs://bucket/file.txt", Staged: true}}
	s, err := devservertest.NewServer(devservertest.Files(files))
	if err != nil {
		t.Fatal("NewServer: ", err)
	}
	defer s.Close()

	// readStaged queries /static for a staged file and returns the status code.
	// If negotiate is true, it adds a "Negotiate: vlist" header to the request.
	readStaged := func(negotiate bool) int {
		u := s.URL + "/static/file.txt?gs_bucket=bucket"
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			t.Fatal("http.NewRequest: ", err)
		}
		if negotiate {
			req.Header.Add("Negotiate", "vlist")
		}

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal("http.Do: ", err)
		}
		defer res.Body.Close()
		return res.StatusCode
	}

	// /static should return 400 if a request doesn't contain "Negotiate: vlist".
	if status := readStaged(true); status != 200 {
		t.Errorf("readStaged(true) = %d; want 200", status)
	}
	if status := readStaged(false); status != 400 {
		t.Errorf("readStaged(false) = %d; want 400", status)
	}
}

func TestPathEscape(t *testing.T) {
	files := []*devservertest.File{{URL: "gs://bucket/path/to/some%20file%2521"}}
	s, err := devservertest.NewServer(devservertest.Files(files))
	if err != nil {
		t.Fatal("NewServer: ", err)
	}
	defer s.Close()

	res, err := http.Get(s.URL + "/stage?archive_url=gs://bucket/path/to&files=some%20file%2521")
	if err != nil {
		t.Fatal("http.Get: ", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Error("/stage failed: ", res.Status)
	}

	res, err = http.Get(s.URL + "/is_staged?archive_url=gs://bucket/path/to&files=some%20file%2521")
	if err != nil {
		t.Fatal("http.Get: ", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Error("/is_staged failed: ", res.Status)
	} else {
		b, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Error("ioutil.ReadAll: ", err)
		} else if string(b) != "True" {
			t.Errorf(`/is_staged returned %q; want "True"`, string(b))
		}
	}

	req, err := http.NewRequest(http.MethodGet, s.URL+"/static/path/to/some%20file%2521?gs_bucket=bucket", nil)
	if err != nil {
		t.Fatal("http.NewRequest: ", err)
	}
	req.Header.Add("Negotiate", "vlist")

	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal("http.Do: ", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Error("/static failed: ", res.Status)
	}
}
