// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver

import (
	"bytes"
	"context"
	"testing"
)

func TestFakeClient(t *testing.T) {
	const (
		url      = "some_url"
		expected = "some_data"
	)

	cl := NewFakeClient(map[string][]byte{
		url: []byte(expected),
	})

	var buf bytes.Buffer
	n, err := cl.DownloadGS(context.Background(), &buf, url)
	if err != nil {
		t.Error("DownloadGS failed: ", err)
	} else if data := buf.String(); data != expected {
		t.Errorf("DownloadGS returned %q; want %q", data, expected)
	} else if n != int64(len(expected)) {
		t.Errorf("DownloadGS returned %d; want %d", n, len(expected))
	}

	if _, err := cl.DownloadGS(context.Background(), &buf, "wrong_url"); err == nil {
		t.Error("DownloadGS unexpectedly succeeded")
	}
}
