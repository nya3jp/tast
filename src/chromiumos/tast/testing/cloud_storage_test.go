// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"chromiumos/tast/internal/devserver"
)

func TestNewCloudStorage(t *testing.T) {
	cs := NewCloudStorage(nil)
	if cs == nil {
		t.Error("NewCloudStorage returned nil")
	}
}

func TestCloudStorageOpen(t *testing.T) {
	const (
		fakeURL     = "gs://a/b/c"
		fakeContent = "hello"
	)

	// Create CloudStorage using a fake devserver client.
	cs := &CloudStorage{
		newClient: func(ctx context.Context) devserver.Client {
			return devserver.NewFakeClient(map[string][]byte{
				fakeURL: []byte(fakeContent),
			})
		},
	}

	r, err := cs.Open(context.Background(), fakeURL)
	if err != nil {
		t.Fatalf("Open failed for %q: %v", fakeURL, err)
	}
	defer r.Close()
	b, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatal("ReadAll failed: ", err)
	}
	if s := string(b); s != fakeContent {
		t.Fatalf("Got content %q, want %q", s, fakeContent)
	}
}

func TestCloudStorageOpenMissing(t *testing.T) {
	// Create CloudStorage using a fake devserver client.
	cs := &CloudStorage{
		newClient: func(ctx context.Context) devserver.Client {
			return devserver.NewFakeClient(nil)
		},
	}

	r, err := cs.Open(context.Background(), "gs://a/b/c")
	if err == nil {
		r.Close()
		t.Fatal("Open succeeded unexpectedly")
	}
	if !os.IsNotExist(err) {
		t.Fatal("Open failed: ", err)
	}
}
