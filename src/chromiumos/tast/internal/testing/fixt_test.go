// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/jsonprotocol"
)

func TestFixtureEntityInfo(t *testing.T) {
	fixt := &Fixture{
		Name:        "chrome.LoggedIn",
		Desc:        "Make sure logged into a Chrome session",
		Contacts:    []string{"a@example.com", "b@example.com"},
		Parent:      "system.Booted",
		ServiceDeps: []string{"chrome.Service"},
	}
	got := fixt.EntityInfo()
	want := &jsonprotocol.EntityInfo{
		Name:        "chrome.LoggedIn",
		Pkg:         "",
		Desc:        "Make sure logged into a Chrome session",
		Contacts:    []string{"a@example.com", "b@example.com"},
		ServiceDeps: []string{"chrome.Service"},
		Fixture:     "system.Booted",
		Type:        jsonprotocol.EntityFixture,
		Bundle:      filepath.Base(os.Args[0]),
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("EntityInfo(%#v) mismatch (-got +want):\n%s", fixt, diff)
	}
}
