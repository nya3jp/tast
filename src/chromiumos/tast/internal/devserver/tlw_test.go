// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver_test

import (
	"context"
	"io/ioutil"
	"testing"

	"chromiumos/tast/internal/devserver"
	"chromiumos/tast/internal/faketlw"
)

func TestTLWClient(t *testing.T) {
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
