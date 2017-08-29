// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"io"
	"net/http"
	"net/http/httptest"

	"chromiumos/tast/common/testing"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/dbusutil"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ChromeSanity,
		Desc: "Checks that Chrome is mostly working",
		Attr: []string{"bvt", "chrome"},
	})
}

func ChromeSanity(s *testing.State) {
	// Start listening for a "started" SessionStateChanged D-Bus signal from session_manager.
	sw, err := dbusutil.NewSignalWatcherForSystemBus(s.Context(), dbusutil.MatchSpec{
		Type:      "signal",
		Path:      dbusutil.SessionManagerPath,
		Interface: dbusutil.SessionManagerInterface,
		Member:    "SessionStateChanged",
		Arg0:      "started",
	})
	if err != nil {
		s.Fatal("Failed to watch for D-Bus signals: ", err)
	}
	defer sw.Close()

	cr, err := chrome.New(s.Context())
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(s.Context())

	s.Log("Waiting for SessionStateChanged \"started\" D-Bus signal from session_manager")
	select {
	case <-sw.Signals:
		s.Log("Got SessionStateChanged signal")
	case <-s.Context().Done():
		s.Fatal("Didn't get SessionStateChanged signal: ", s.Context().Err())
	}

	conn, err := cr.NewConn(s.Context(), "")
	if err != nil {
		s.Fatal(err)
	}
	defer conn.Close()

	const expected = "Hooray, it worked!"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, expected)
	}))
	defer server.Close()

	if err = conn.Navigate(s.Context(), server.URL); err != nil {
		s.Fatal(err)
	}
	var actual string
	if err = conn.Eval(s.Context(), "document.documentElement.innerText", &actual); err != nil {
		s.Fatal(err)
	}
	s.Logf("Got content %q", actual)
	if actual != expected {
		s.Fatalf("Expected page content %q, got %q", expected, actual)
	}
}
