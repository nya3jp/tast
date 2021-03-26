// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestEntity(t *testing.T) {
	want := &Entity{Data: []string{"data"}}

	test := &TestInstance{Data: []string{"data"}}
	fixt := &Fixture{Data: []string{"data"}}

	if diff := cmp.Diff(test.Entity(), want); diff != "" {
		t.Errorf("test entity mismatch (-got +want):\n%v", diff)
	}
	if diff := cmp.Diff(fixt.Entity(), want); diff != "" {
		t.Errorf("fixture entity mismatch (-got +want):\n%v", diff)
	}
}
