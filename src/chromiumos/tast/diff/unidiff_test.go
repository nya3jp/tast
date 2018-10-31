// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package diff

import "testing"

func TestDiff(t *testing.T) {
	for _, data := range []struct {
		in1, in2 []byte
		expect   string
	}{
		{nil, nil, ""},
		{[]byte(""), []byte(""), ""},
		{[]byte("a"), []byte("a"), ""},
		{[]byte("a"), []byte("b"), "@@ -1 +1 @@\n-a\n\\ No newline at end of file\n+b\n\\ No newline at end of file\n"},
		{[]byte("a\nb\nc\n"), []byte("a\nx\nc\n"), "@@ -1,3 +1,3 @@\n a\n-b\n+x\n c\n"},
	} {
		d, err := Diff(data.in1, data.in2)
		if err != nil {
			t.Errorf("Failed to run Diff(%v, %v): %v", data.in1, data.in2, err)
		} else if d != data.expect {
			t.Errorf("Diff(%v, %v) produced %q; want %q", data.in1, data.in2, d, data.expect)
		}
	}
}
