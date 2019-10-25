// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"fmt"
	"testing"
)

func TestFirstLineBuffer(t *testing.T) {
	const (
		first  = "first line"
		second = "second line"
	)
	var b firstLineBuffer
	fmt.Fprintf(&b, "%s\n%s\n", first, second)

	if ln := b.FirstLine(); ln != first {
		t.Errorf("FirstLine() = %q; want %q", ln, first)
	}
}

func TestFirstLineBufferEmpty(t *testing.T) {
	var b firstLineBuffer
	if ln := b.FirstLine(); ln != "" {
		t.Errorf("FirstLine() = %q; want %q", ln, "")
	}
}
