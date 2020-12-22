// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"
	"testing"

	"chromiumos/tast/cmd/tast_rtd/internal/fakerts"
	"chromiumos/tast/errors"
)

func TestSendLog(t *testing.T) {
	srv, addr, err := fakerts.StartProgressSink(context.Background())
	if err != nil {
		t.Fatal("Failed to start fake ProgressSink server: ", err)
	}
	defer srv.Stop()
	name := "file://foo/name001"
	reqName1 := "request01"
	reqName2 := "request02"
	log, err := newReportLogStream(addr.String(), name)
	if err != nil {
		t.Fatal(err)
	}

	p1 := []byte("Hello, ")
	p2 := []byte("world!")
	if err := log.Write(reqName1, p1); err != nil {
		t.Error(errors.Wrapf(err, "failed to write first log"))
	}
	if err := log.Write(reqName1, p2); err != nil {
		t.Error(errors.Wrapf(err, "failed to write second log"))
	}
	if err := log.Write(reqName2, p1); err != nil {
		t.Error(errors.Wrapf(err, "failed to write third log"))
	}
	log.Close()
	actual := string(srv.ReceivedLog(name, reqName1))
	expected := "Hello, world!"
	if actual != expected {
		t.Errorf("got %q, want %q", actual, expected)
	}
	actual = string(srv.ReceivedLog(name, reqName2))
	expected = "Hello, "
	if actual != expected {
		t.Errorf("got %q, want %q", actual, expected)
	}
}
