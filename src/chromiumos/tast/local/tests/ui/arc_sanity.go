// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"chromiumos/tast/common/testing"
	"chromiumos/tast/local/chrome"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ARCSanity,
		Desc: "Checks that ARC starts",
		Attr: []string{"bvt", "chrome", "arc"},
	})
}

func ARCSanity(s *testing.State) {
	cr, err := chrome.New(s.Context(), chrome.ARCEnabled())
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(s.Context())

	// TODO(derat): Do more to test that ARC is working.
}
