// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing_test

import (
	gotesting "testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
)

func TestFixtureEntityProto(t *gotesting.T) {
	fixt := &testing.FixtureInstance{
		Pkg:         "pkg",
		Name:        "chrome.LoggedIn",
		Desc:        "Make sure logged into a Chrome session",
		Contacts:    []string{"a@example.com", "b@example.com"},
		Parent:      "system.Booted",
		Data:        []string{"data.txt"},
		ServiceDeps: []string{"chrome.Service"},
		Bundle:      "bundle",
	}
	got := fixt.EntityProto()
	want := &protocol.Entity{
		Type:        protocol.EntityType_FIXTURE,
		Name:        "chrome.LoggedIn",
		Package:     "pkg",
		Description: "Make sure logged into a Chrome session",
		Fixture:     "system.Booted",
		Dependencies: &protocol.EntityDependencies{
			DataFiles: []string{"data.txt"},
			Services:  []string{"chrome.Service"},
		},
		Contacts: &protocol.EntityContacts{
			Emails: []string{"a@example.com", "b@example.com"},
		},
		LegacyData: &protocol.EntityLegacyData{
			Bundle: "bundle",
		},
	}
	if diff := cmp.Diff(got, want, protocmp.Transform()); diff != "" {
		t.Errorf("EntityProto(%#v) mismatch (-got +want):\n%s", fixt, diff)
	}
}
