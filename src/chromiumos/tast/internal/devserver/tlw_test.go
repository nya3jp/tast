// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver_test

import (
	"context"
	"io/ioutil"
	"net/http"
	"testing"

	"chromiumos/tast/internal/devserver"
	"chromiumos/tast/internal/faketlw"
)

func TestTLWClientOpen(t *testing.T) {
	const gsURL = "gs://bucket/path/to/some%20file%2521"
	const content = "abc"
	const dutName = "dut001"

	stopFunc, addr := faketlw.StartWiringServer(t, faketlw.WithCacheFileMap(
		map[string][]byte{
			gsURL: []byte(content),
		},
	), faketlw.WithDUTName(dutName))
	defer stopFunc()

	cl, err := devserver.NewTLWClient(context.Background(), addr, dutName)
	if err != nil {
		t.Fatal("Failed to create TLW client: ", err)
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

func TestTLWClientStage(t *testing.T) {
	const gsURL = "gs://bucket/path/to/some%20file%2521"
	const content = "abc"
	const dutName = "dut001"

	stopFunc, addr := faketlw.StartWiringServer(t, faketlw.WithCacheFileMap(
		map[string][]byte{
			gsURL: []byte(content),
		},
	), faketlw.WithDUTName(dutName))
	defer stopFunc()

	cl, err := devserver.NewTLWClient(context.Background(), addr, dutName)
	if err != nil {
		t.Fatal("Failed to create TLW client: ", err)
	}
	defer cl.TearDown()

	fileURL, err := cl.Stage(context.Background(), gsURL)
	if err != nil {
		t.Fatal("Stage failed: ", err)
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
	} else if string(data) != content {
		t.Errorf("Get returned %q; want %q", string(data), content)
	}

	if fileURL, err := cl.Stage(context.Background(), "gs://bucket/path/to/wrong_file"); err == nil {
		t.Error("Stage unexpectedly succeeded: ", fileURL)
	}
}
