// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package diff_test

import (
	"testing"

	"go.chromium.org/tast/cmd/tast-lint/internal/diff"
)

func TestDiff(t *testing.T) {
	for _, data := range []struct{ in1, in2, expect string }{
		{"", "", ""},
		{"a", "a", ""},
		{"a", "b", "@@ -1 +1 @@\n-a\n\\ No newline at end of file\n+b\n\\ No newline at end of file\n"},
		{"a\nb\nc\n", "a\nx\nc\n", "@@ -1,3 +1,3 @@\n a\n-b\n+x\n c\n"},
	} {
		d, err := diff.Diff(data.in1, data.in2)
		if err != nil {
			t.Errorf("Failed to run Diff(%q, %q): %v", data.in1, data.in2, err)
		} else if d != data.expect {
			t.Errorf("Diff(%q, %q) produced %q; want %q", data.in1, data.in2, d, data.expect)
		}
	}
}
