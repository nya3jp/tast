// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"
	"testing"

	"chromiumos/tast/cmd/tast_rtd/internal/fakerts"
)

func TestSendLog(t *testing.T) {
	srv, addr, err := fakerts.StartProgressSink(context.Background())
	if err != nil {
		t.Fatal("Failed to start fake ProgressSink server: ", err)
	}
	defer srv.Stop()
	name := "file://foo/name001"
	reqName := "request01"
	log, err := newReportLogStream(addr.String(), name, reqName)
	if err != nil {
		t.Fatal(err)
	}

	p1 := []byte("Hello, ")
	p2 := []byte("world!")
	if _, err := log.Write(p1); err != nil {
		t.Error(errors.Wrafp(err, "failed to write first log")
	}
	if _, err := log.Write(p2); err != nil {
		t.Error(errors.Wrafp(err, "failed to write second log")
	}
	log.Close()
	actual := string(srv.ReceivedLog(name, reqName))
	expected := "Hello, world!"
	if actual != expected {
		t.Errorf("got %q, want %q", actual, expected)
	}
}
