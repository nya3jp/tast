// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package set

import (
	"reflect"
	"testing"
)

func TestStringSliceDiff(t *testing.T) {
	before := []string{"a.0.dmp", "a.1.dmp", "a.2.dmp"}
	after := []string{"a.0.dmp", "a.1.dmp", "b.0.dmp", "c.0.dmp"}

	justNew := StringSliceDiff(before, after)
	expected := []string{"b.0.dmp", "c.0.dmp"}
	if !reflect.DeepEqual(justNew, expected) {
		t.Errorf("Unexpected files: got %v, want %v", justNew, expected)
	}
}
