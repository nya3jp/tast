// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	gotesting "testing"
)

func TestAddFixture_Package(t *gotesting.T) {
	defer SetGlobalRegistryForTesting(NewRegistry())()

	AddFixture(&Fixture{Name: "foo"})
	if errs := RegistrationErrors(); errs != nil {
		t.Fatalf("AddFixture: %v", errs)
	}
	if got, want := GlobalRegistry().AllFixtures()["foo"].pkg, "chromiumos/tast/internal/testing"; got != want {
		t.Errorf("pkg mismatch; got %q, want %q", got, want)
	}
}

func TestAddFixture_DuplicateName(t *gotesting.T) {
	defer SetGlobalRegistryForTesting(NewRegistry())()

	const name = "foo"
	AddFixture(&Fixture{Name: name})
	if errs := RegistrationErrors(); errs != nil {
		t.Fatalf("Fixture registration failed: %v", errs)
	}
	AddFixture(&Fixture{Name: name})
	if errs := RegistrationErrors(); errs == nil {
		t.Error("Duplicated fixture registration succeeded unexpectedly")
	}
}

func TestAddFixture_InvalidName(t *gotesting.T) {
	for _, tc := range []struct {
		name string
		ok   bool
	}{
		{"", false},
		{"a", true},
		{"A", false},
		{"1", false},
		{"%", false},
		{"abc", true},
		{"aBc", true},
		{"a1r", true},
		{"a1R", true},
		{"a r", false},
		{"a_r", false},
		{"a-r", false},
		{"chromeLoggedIn", true},
		{"ieee1394", true},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			defer SetGlobalRegistryForTesting(NewRegistry())()

			AddFixture(&Fixture{Name: tc.name})
			errs := RegistrationErrors()
			if tc.ok && errs != nil {
				t.Errorf("AddFixture(%q) failed: %v", tc.name, errs)
			}
			if !tc.ok && errs == nil {
				t.Errorf("AddFixture(%q) passed unexpectedly", tc.name)
			}
		})
	}
}
