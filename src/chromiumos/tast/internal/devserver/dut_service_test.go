// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver_test

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"chromiumos/tast/internal/devserver"
	"chromiumos/tast/internal/fakedutserver"
)

func TestDUTServiceClientOpen(t *testing.T) {
	const gsURL = "gs://bucket/path/to/some%20file%2521"
	const content = "abc"

	stopFunc, addr := fakedutserver.Start(t, fakedutserver.WithCacheFileMap(
		map[string][]byte{
			gsURL: []byte(content),
		},
	))
	defer stopFunc()

	cl, err := devserver.NewDUTServiceClient(context.Background(), addr)
	if err != nil {
		t.Fatal("Failed to create DUTService client: ", err)
	}
	defer cl.TearDown()

	r, err := cl.Open(context.Background(), gsURL)
	if err != nil {
		t.Fatal("Open failed: ", err)
	}
	defer r.Close()
	if data, err := ioutil.ReadAll(r); err != nil {
		t.Error("ReadAll failed: ", err)
	} else if string(data) != content {
		t.Errorf("Open returned %q; want %q", string(data), content)
	}

	if r, err := cl.Open(context.Background(), "gs://bucket/path/to/wrong_file"); err == nil {
		r.Close()
		t.Error("Open unexpectedly succeeded")
	}
}

func TestDUTServiceClientStage(t *testing.T) {
	const gsURL = "gs://bucket/path/to/some%20file%2521"
	const content = "abc"

	stopFunc, addr := fakedutserver.Start(t, fakedutserver.WithCacheFileMap(
		map[string][]byte{
			gsURL: []byte(content),
		},
	))
	defer stopFunc()

	cl, err := devserver.NewDUTServiceClient(context.Background(), addr)
	if err != nil {
		t.Fatal("Failed to create DUTService client: ", err)
	}
	defer cl.TearDown()

	fileURL, err := cl.Stage(context.Background(), gsURL)
	if err != nil {
		t.Fatal("Stage failed: ", err)
	}
	if fileURL.Scheme != "file" {
		t.Fatal("Expected file url, got ", fileURL)
	}
	r, err := os.Open(fileURL.Path)
	if err != nil {
		t.Fatalf("Get URL %q failed: %s", fileURL, err)
	}
	defer r.Close()
	if data, err := ioutil.ReadAll(r); err != nil {
		t.Error("ReadAll failed: ", err)
	} else if string(data) != content {
		t.Errorf("Open returned %q; want %q", string(data), content)
	}

	if _, err := cl.Stage(context.Background(), "gs://bucket/path/to/wrong_file"); err == nil {
		t.Error("Open unexpectedly succeeded")
	}
}
