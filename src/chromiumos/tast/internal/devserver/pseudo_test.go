// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestPseudoClient(t *testing.T) {
	const expected = "some_data"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bucket/path/to/some file%21" {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, expected)
	}))
	defer server.Close()

	cl := NewPseudoClient(nil)
	cl.url = server.URL

	var buf bytes.Buffer
	n, err := cl.DownloadGS(context.Background(), &buf, "gs://bucket/path/to/some%20file%2521")
	if err != nil {
		t.Error("DownloadGS failed: ", err)
	} else if data := buf.String(); data != expected {
		t.Errorf("DownloadGS returned %q; want %q", data, expected)
	} else if n != int64(len(expected)) {
		t.Errorf("DownloadGS returned %d; want %d", n, len(expected))
	}

	if _, err := cl.DownloadGS(context.Background(), ioutil.Discard, "gs://bucket/path/to/wrong_file"); err == nil {
		t.Error("DownloadGS unexpectedly succeeded")
	} else if !os.IsNotExist(err) {
		t.Errorf("DownloadGS returned %q; want %q", err, os.ErrNotExist)
	}
}
