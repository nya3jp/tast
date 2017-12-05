// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package build

import (
	"reflect"
	"testing"
)

func TestParseEqueryDeps(t *testing.T) {
	// Copy-and-pasted output (including trailing whitespace) from
	// "equery -q -C g --depth=1 chromeos-base/tast-local-tests-9999".
	out := `
chromeos-base/tast-local-tests-9999:
 [  0]  chromeos-base/tast-local-tests-9999   
 [  1]  chromeos-base/tast-common-9999   
 [  1]  dev-go/cdp-0.9.1   
 [  1]  dev-go/dbus-0.0.2-r5   
 [  1]  dev-lang/go-1.8.3-r1   
 [  1]  dev-vcs/git-2.12.2   
`

	exp := []string{
		"chromeos-base/tast-common-9999",
		"dev-go/cdp-0.9.1",
		"dev-go/dbus-0.0.2-r5",
		"dev-lang/go-1.8.3-r1",
		"dev-vcs/git-2.12.2",
	}
	if act := parseEqueryDeps([]byte(out)); !reflect.DeepEqual(act, exp) {
		t.Errorf("parseEqueryDeps(%q) = %v; want %v", out, act, exp)
	}
}
