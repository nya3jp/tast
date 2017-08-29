// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"chromiumos/tast/common/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Fail,
		Desc: "Always fails",
	})
}

func Fail(s *testing.State) {
	s.Log("Here's an informative message")
	s.Error("Here's an error")
	s.Error("And here's a second")
	s.Fatal("Finally, a fatal error")
}
