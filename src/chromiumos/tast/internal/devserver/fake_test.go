// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver_test

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"chromiumos/tast/internal/devserver"
)

func TestFakeClient(t *testing.T) {
	const (
		url      = "some_url"
		expected = "some_data"
	)

	cl := devserver.NewFakeClient(map[string][]byte{
		url: []byte(expected),
	})

	r, err := cl.Open(context.Background(), url)
	if err != nil {
		t.Error("Open failed: ", err)
	}
	defer r.Close()
	if data, err := ioutil.ReadAll(r); err != nil {
		t.Error("ReadAll failed: ", err)
	} else if string(data) != expected {
		t.Errorf("Open returned %q; want %q", string(data), expected)
	}

	if r, err := cl.Open(context.Background(), "wrong_url"); err == nil {
		r.Close()
		t.Error("Open unexpectedly succeeded")
	} else if !os.IsNotExist(err) {
		t.Errorf("Open returned %q; want %q", err, os.ErrNotExist)
	}
}
