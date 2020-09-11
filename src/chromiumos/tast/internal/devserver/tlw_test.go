// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver_test

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"chromiumos/tast/internal/devserver"
	"chromiumos/tast/internal/faketlw"
)

func TestTlwClient(t *testing.T) {
	const expected = "This is data file."
	const gsURL = "gs://bucket/path/to/some%20file%2521"

	stopFunc, addr, err := faketlw.StartWiringServer(t, faketlw.WithCacheFileMap(
		map[string]string{
			gsURL: "abc",
		},
	))
	if err != nil {
		t.Fatal("Failed to start fake Wiring service: ", err)
	}
	defer stopFunc()

	cl, err := devserver.NewTLWClient(context.Background(), addr)
	if err != nil {
		t.Fatal("Failed to create TLW client: ", err)
	}

	r, err := cl.Open(context.Background(), gsURL)
	if err != nil {
		t.Fatal("Open failed: ", err)
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
