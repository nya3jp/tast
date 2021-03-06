// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"testing"

	"chromiumos/tast/internal/devserver"
	"chromiumos/tast/internal/faketlw"
)

func TestNewCloudStorage(t *testing.T) {
	cs := NewCloudStorage(nil, "", "")
	if cs == nil {
		t.Error("NewCloudStorage returned nil")
	}
}

func TestNewCloudStorageTLW(t *testing.T) {
	stopFunc, addr := faketlw.StartWiringServer(t)
	defer stopFunc()
	cs := NewCloudStorage(nil, addr, "dutName001")
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
		newClient: func(ctx context.Context) (devserver.Client, error) {
			return devserver.NewFakeClient(map[string][]byte{
				fakeURL: []byte(fakeContent),
			}), nil
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

func TestCloudStorageTLWOpen(t *testing.T) {
	const (
		dutName     = "dut001"
		fakeURL     = "gs://a/b/c"
		fakeContent = "fake content"
	)
	stopFunc, addr := faketlw.StartWiringServer(t, faketlw.WithCacheFileMap(
		map[string][]byte{
			fakeURL: []byte(fakeContent),
		},
	), faketlw.WithDUTName(dutName))
	defer stopFunc()

	// Create CloudStorage using a fake TLW client.
	cs := &CloudStorage{
		newClient: func(ctx context.Context) (devserver.Client, error) {
			return devserver.NewTLWClient(ctx, addr, dutName)
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
		newClient: func(ctx context.Context) (devserver.Client, error) {
			return devserver.NewFakeClient(nil), nil
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

func TestCloudStorageTLWOpenMissing(t *testing.T) {
	const dutName = "dut001"
	stopFunc, addr := faketlw.StartWiringServer(t, faketlw.WithDUTName(dutName)) // no file served
	defer stopFunc()

	// Create CloudStorage using a fake TLW client.
	cs := &CloudStorage{
		newClient: func(ctx context.Context) (devserver.Client, error) {
			return devserver.NewTLWClient(ctx, addr, dutName)
		},
	}

	r, err := cs.Open(context.Background(), "gs://a/b/c")
	if err == nil {
		r.Close()
		t.Fatal("Open succeeded unexpectedly")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Open failed with unexpected error: ", err)
	}
}
