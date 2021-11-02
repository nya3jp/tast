// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crosbundle

import (
	"reflect"
	"testing"

	"chromiumos/tast/autocaps"
)

func TestDetermineSoftwareFeatures(t *testing.T) {
	defs := map[string]string{"a": "foo && bar", "b": "foo && baz"}
	flags := []string{"foo", "bar"}
	autotestCaps := map[string]autocaps.State{"c": autocaps.Yes, "d": autocaps.No, "e": autocaps.Disable}
	features, err := determineSoftwareFeatures(defs, flags, autotestCaps)
	if err != nil {
		t.Fatalf("determineSoftwareFeatures(%v, %v, %v) failed: %v", defs, flags, autotestCaps, err)
	}
	if exp := []string{"a", autotestCapPrefix + "c"}; !reflect.DeepEqual(features.Available, exp) {
		t.Errorf("determineSoftwareFeatures(%v, %v, %v) returned available features %v; want %v",
			defs, flags, autotestCaps, features.Available, exp)
	}
	if exp := []string{autotestCapPrefix + "d", autotestCapPrefix + "e", "b"}; !reflect.DeepEqual(features.Unavailable, exp) {
		t.Errorf("determineSoftwareFeatures(%v, %v, %v) returned unavailable features %v; want %v",
			defs, flags, autotestCaps, features.Unavailable, exp)
	}
}
