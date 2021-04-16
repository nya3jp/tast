// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package jsonprotocol_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/protocol"
)

func TestFeatureArgsFeatures(t *testing.T) {
	j := &jsonprotocol.FeatureArgs{
		CheckDeps: false,
		TestVars:  map[string]string{"a": "b"},
	}

	got := j.Features()

	// Even if CheckDeps is false, vars is filled.
	want := &protocol.Features{
		CheckDeps: false,
		Vars:      map[string]string{"a": "b"},
		Software:  &protocol.SoftwareFeatures{},
		Hardware:  &protocol.HardwareFeatures{},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("Features conversion mismatch (-got +want):\n%s", diff)
	}
}
