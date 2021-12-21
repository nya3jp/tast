// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver_test

import (
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"chromiumos/tast/internal/devserver"
)

func TestPseudoClientOpen(t *testing.T) {
	const expected = "some_data"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bucket/path/to/some file%21" {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, expected)
	}))
	defer server.Close()

	cl := devserver.NewPseudoClient(devserver.WithBaseURL(server.URL))

	r, err := cl.Open(context.Background(), "gs://bucket/path/to/some%20file%2521")
	if err != nil {
		t.Error("Open failed: ", err)
	}
	defer r.Close()
	if data, err := ioutil.ReadAll(r); err != nil {
		t.Error("ReadAll failed: ", err)
	} else if string(data) != expected {
		t.Errorf("Open returned %q; want %q", string(data), expected)
	}

	if r, err := cl.Open(context.Background(), "gs://bucket/path/to/wrong_file"); err == nil {
		r.Close()
		t.Error("Open unexpectedly succeeded")
	} else if !os.IsNotExist(err) {
		t.Errorf("Open returned %q; want %q", err, os.ErrNotExist)
	}
}

func TestPseudoClientStage(t *testing.T) {
	const expected = "some_data"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bucket/path/to/some file%21" {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, expected)
	}))
	defer server.Close()

	cl := devserver.NewPseudoClient(devserver.WithBaseURL(server.URL))

	fileURL, err := cl.Stage(context.Background(), "gs://bucket/path/to/some%20file%2521")
	if err != nil {
		t.Error("Stage failed: ", err)
	}
	resp, err := http.Get(fileURL.String())
	if err != nil {
		t.Error("Get failed: ", err)
	}
	if resp.StatusCode != 200 {
		t.Error("Get failed: ", resp)
	}
	defer resp.Body.Close()
	if data, err := ioutil.ReadAll(resp.Body); err != nil {
		t.Error("ReadAll failed: ", err)
	} else if string(data) != expected {
		t.Errorf("Get returned %q; want %q", string(data), expected)
	}

	fileURL, err = cl.Stage(context.Background(), "gs://bucket/path/to/wrong_file")
	if err != nil {
		t.Error("Stage failed: ", err)
	}
	resp, err = http.Get(fileURL.String())
	if err != nil {
		t.Error("Get failed: ", err)
	}
	if resp.StatusCode != 404 {
		t.Errorf("Get returned %q; want %q", resp.Status, 404)
	}
}
