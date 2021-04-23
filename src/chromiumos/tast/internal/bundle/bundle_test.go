// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/jsonprotocol"
	"chromiumos/tast/internal/testing"
)

func TestRunList(t *gotesting.T) {
	reg := testing.NewRegistry()

	f := func(context.Context, *testing.State) {}
	tests := []*testing.TestInstance{
		{Name: "pkg.Test", Func: f},
		{Name: "pkg.Test2", Func: f},
	}

	for _, test := range tests {
		reg.AddTestInstance(test)
	}

	var infos []*jsonprotocol.EntityWithRunnabilityInfo
	for _, test := range tests {
		infos = append(infos, &jsonprotocol.EntityWithRunnabilityInfo{
			EntityInfo: *jsonprotocol.MustEntityInfoFromProto(test.EntityProto()),
		})
	}

	var exp bytes.Buffer
	if err := json.NewEncoder(&exp).Encode(infos); err != nil {
		t.Fatal(err)
	}

	// BundleListTestsMode should result in tests being JSON-marshaled to stdout.
	stdin := newBufferWithArgs(t, &jsonprotocol.BundleArgs{Mode: jsonprotocol.BundleListTestsMode, ListTests: &jsonprotocol.BundleListTestsArgs{}})
	stdout := &bytes.Buffer{}
	if status := run(context.Background(), nil, stdin, stdout, &bytes.Buffer{}, NewStaticConfig(reg, 0, Delegate{})); status != statusSuccess {
		t.Fatalf("run() returned status %v; want %v", status, statusSuccess)
	}
	if stdout.String() != exp.String() {
		t.Errorf("run() wrote %q; want %q", stdout.String(), exp.String())
	}

	// The -dumptests command-line flag should do the same thing.
	clArgs := []string{"-dumptests"}
	stdout.Reset()
	if status := run(context.Background(), clArgs, &bytes.Buffer{}, stdout, &bytes.Buffer{}, NewStaticConfig(reg, 0, Delegate{})); status != statusSuccess {
		t.Fatalf("run(%v) returned status %v; want %v", clArgs, status, statusSuccess)
	}
	if stdout.String() != exp.String() {
		t.Errorf("run(%v) wrote %q; want %q", clArgs, stdout.String(), exp.String())
	}
}

// TestRunListWithDep tests run.run for listing test with dependency check.
func TestRunListWithDep(t *gotesting.T) {
	const (
		validDep   = "valid"
		missingDep = "missing"
	)

	reg := testing.NewRegistry()

	f := func(context.Context, *testing.State) {}
	tests := []*testing.TestInstance{
		{Name: "pkg.Test", Func: f, SoftwareDeps: []string{validDep}},
		{Name: "pkg.Test2", Func: f, SoftwareDeps: []string{missingDep}},
	}

	expectedPassTests := map[string]struct{}{tests[0].Name: struct{}{}}
	expectedSkipTests := map[string]struct{}{tests[1].Name: struct{}{}}

	for _, test := range tests {
		reg.AddTestInstance(test)
	}

	args := jsonprotocol.BundleArgs{
		Mode: jsonprotocol.BundleListTestsMode,
		ListTests: &jsonprotocol.BundleListTestsArgs{
			FeatureArgs: jsonprotocol.FeatureArgs{
				CheckDeps:                   true,
				TestVars:                    map[string]string{},
				AvailableSoftwareFeatures:   []string{validDep},
				UnavailableSoftwareFeatures: []string{missingDep},
			},
		},
	}

	// BundleListTestsMode should result in tests being JSON-marshaled to stdout.
	stdin := newBufferWithArgs(t, &args)
	stdout := &bytes.Buffer{}
	if status := run(context.Background(), nil, stdin, stdout, &bytes.Buffer{}, NewStaticConfig(reg, 0, Delegate{})); status != statusSuccess {
		t.Fatalf("run() returned status %v; want %v", status, statusSuccess)
	}
	var ts []jsonprotocol.EntityWithRunnabilityInfo
	if err := json.Unmarshal(stdout.Bytes(), &ts); err != nil {
		t.Fatalf("unmarshal output %q: %v", stdout.String(), err)
	}
	if len(ts) != len(tests) {
		t.Fatalf("run() returned %v entities; want %v", len(ts), len(tests))
	}
	for _, test := range ts {
		if _, ok := expectedPassTests[test.Name]; ok {
			if test.SkipReason != "" {
				t.Fatalf("run() returned test %q with skip reason %q; want none", test.Name, test.SkipReason)
			}
		}
		if _, ok := expectedSkipTests[test.Name]; ok {
			if test.SkipReason == "" {
				t.Fatalf("run() returned test %q with no skip reason; want %q", test.Name, test.SkipReason)
			}
		}
	}
}

func TestRunListFixtures(t *gotesting.T) {
	reg := testing.NewRegistry()

	fixts := []*testing.Fixture{
		{Name: "b", Parent: "a"},
		{Name: "c"},
		{Name: "d"},
		{Name: "a"},
	}

	for _, f := range fixts {
		reg.AddFixture(f)
	}

	// BundleListFixturesMode should output JSON-marshaled fixtures to stdout.
	stdin := newBufferWithArgs(t, &jsonprotocol.BundleArgs{Mode: jsonprotocol.BundleListFixturesMode})
	stdout := &bytes.Buffer{}
	if status := run(context.Background(), nil, stdin, stdout, &bytes.Buffer{}, NewStaticConfig(reg, 0, Delegate{})); status != statusSuccess {
		t.Fatalf("run() = %v, want %v", status, statusSuccess)
	}

	got := make([]*jsonprotocol.EntityInfo, 0)
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(%q): %v", stdout.String(), err)
	}
	bundle := filepath.Base(os.Args[0])
	want := []*jsonprotocol.EntityInfo{
		{Type: jsonprotocol.EntityFixture, Name: "a", Bundle: bundle},
		{Type: jsonprotocol.EntityFixture, Name: "b", Fixture: "a", Bundle: bundle},
		{Type: jsonprotocol.EntityFixture, Name: "c", Bundle: bundle},
		{Type: jsonprotocol.EntityFixture, Name: "d", Bundle: bundle},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("Output mismatch (-got +want): %v", diff)
	}
}

func TestRunRegistrationError(t *gotesting.T) {
	reg := testing.NewRegistry()
	const name = "cat.MyTest"
	reg.AddTestInstance(&testing.TestInstance{Name: name, Func: testFunc})

	// Adding a test with same name should generate an error.
	reg.AddTestInstance(&testing.TestInstance{Name: name, Func: testFunc})

	stdin := newBufferWithArgs(t, &jsonprotocol.BundleArgs{Mode: jsonprotocol.BundleListTestsMode, ListTests: &jsonprotocol.BundleListTestsArgs{}})
	if status := run(context.Background(), nil, stdin, ioutil.Discard, ioutil.Discard, NewStaticConfig(reg, 0, Delegate{})); status != statusBadTests {
		t.Errorf("run() with bad test returned status %v; want %v", status, statusBadTests)
	}
}
