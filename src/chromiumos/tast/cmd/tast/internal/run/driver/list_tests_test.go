// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver_test

import (
	"context"
	gotesting "testing"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/driver"
	"chromiumos/tast/cmd/tast/internal/run/runtest"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
)

func newDriverForListingTests(t *gotesting.T) (context.Context, *driver.Driver, *protocol.Features) {
	local1 := testing.NewRegistry("bundle1")
	local1.AddTestInstance(&testing.TestInstance{Name: "pkg.Local1", Attr: []string{"yes"}})
	local1.AddTestInstance(&testing.TestInstance{Name: "pkg.Local2", Attr: []string{"no"}})
	local1.AddFixtureInstance(&testing.FixtureInstance{Name: "fixt.LocalA", Parent: "fixt.RemoteA"})
	local1.AddFixtureInstance(&testing.FixtureInstance{Name: "fixt.LocalB", Parent: "fixt.LocalA"})

	remote1 := testing.NewRegistry("bundle1")
	remote1.AddTestInstance(&testing.TestInstance{Name: "pkg.Remote1", Attr: []string{"no"}})
	remote1.AddTestInstance(&testing.TestInstance{Name: "pkg.Remote2", Attr: []string{"yes"}})
	remote1.AddFixtureInstance(&testing.FixtureInstance{Name: "fixt.RemoteA", Parent: "fixt.RemoteB"})
	remote1.AddFixtureInstance(&testing.FixtureInstance{Name: "fixt.RemoteB"})

	local2 := testing.NewRegistry("bundle2")
	local2.AddTestInstance(&testing.TestInstance{Name: "pkg.Local3", Attr: []string{"yes"}, VarDeps: []string{"var"}})
	local2.AddTestInstance(&testing.TestInstance{Name: "pkg.Local4", Attr: []string{"no"}})
	local2.AddFixtureInstance(&testing.FixtureInstance{Name: "fixt.LocalA", Parent: "fixt.LocalB"})
	local2.AddFixtureInstance(&testing.FixtureInstance{Name: "fixt.LocalB", Parent: "fixt.RemoteB"})

	remote2 := testing.NewRegistry("bundle2")
	remote2.AddTestInstance(&testing.TestInstance{Name: "pkg.Remote3", Attr: []string{"no"}, VarDeps: []string{"var"}})
	remote2.AddTestInstance(&testing.TestInstance{Name: "pkg.Remote4", Attr: []string{"yes"}})
	remote2.AddFixtureInstance(&testing.FixtureInstance{Name: "fixt.RemoteA"})
	remote2.AddFixtureInstance(&testing.FixtureInstance{Name: "fixt.RemoteB", Parent: "fixt.RemoteA"})

	env := runtest.SetUp(t, runtest.WithLocalBundles(local1, local2), runtest.WithRemoteBundles(remote1, remote2))
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.Patterns = []string{"(yes)"}
	})

	drv, err := driver.New(ctx, cfg, cfg.Target())
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	t.Cleanup(func() { drv.Close(ctx) })

	features := &protocol.Features{CheckDeps: true}

	return ctx, drv, features
}

func makeResolvedEntityForTest(bundle string, hops int32, name, attr string, varDeps []string) *protocol.ResolvedEntity {
	return &protocol.ResolvedEntity{
		Entity: &protocol.Entity{
			Type:         protocol.EntityType_TEST,
			Name:         name,
			Attributes:   []string{attr},
			Dependencies: &protocol.EntityDependencies{},
			Contacts:     &protocol.EntityContacts{},
			LegacyData: &protocol.EntityLegacyData{
				Timeout:      ptypes.DurationProto(0),
				Bundle:       bundle,
				VariableDeps: varDeps,
			},
		},
		Hops: hops,
	}
}

func makeResolvedEntityForFixture(bundle string, hops int32, name, parent, start string) *protocol.ResolvedEntity {
	return &protocol.ResolvedEntity{
		Entity: &protocol.Entity{
			Type:         protocol.EntityType_FIXTURE,
			Name:         name,
			Fixture:      parent,
			Dependencies: &protocol.EntityDependencies{},
			Contacts:     &protocol.EntityContacts{},
			LegacyData: &protocol.EntityLegacyData{
				Timeout: ptypes.DurationProto(0),
				Bundle:  bundle,
			},
		},
		Hops:             hops,
		StartFixtureName: start,
	}
}

func TestDriver_ListMatchedTests(t *gotesting.T) {
	ctx, drv, features := newDriverForListingTests(t)

	got, err := drv.ListMatchedTests(ctx, features)
	if err != nil {
		t.Fatal("ListMatchedTests failed: ", err)
	}

	want := []*protocol.ResolvedEntity{
		makeResolvedEntityForTest("bundle1", 1, "pkg.Local1", "yes", nil),
		makeResolvedEntityForTest("bundle2", 1, "pkg.Local3", "yes", []string{"var"}),
		makeResolvedEntityForTest("bundle1", 0, "pkg.Remote2", "yes", nil),
		makeResolvedEntityForTest("bundle2", 0, "pkg.Remote4", "yes", nil),
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("Unexpected list of tests (-got +want):\n%v", diff)
	}
}

func TestDriver_ListMatchedLocalTests(t *gotesting.T) {
	ctx, drv, features := newDriverForListingTests(t)

	got, err := drv.ListMatchedLocalTests(ctx, features)
	if err != nil {
		t.Fatal("ListMatchedLocalTests failed: ", err)
	}

	want := []*protocol.ResolvedEntity{
		makeResolvedEntityForTest("bundle1", 1, "pkg.Local1", "yes", nil),
		makeResolvedEntityForTest("bundle2", 1, "pkg.Local3", "yes", []string{"var"}),
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("Unexpected list of tests (-got +want):\n%v", diff)
	}
}

func TestDriver_ListLocalFixtures(t *gotesting.T) {
	ctx, drv, _ := newDriverForListingTests(t)

	got, err := drv.ListLocalFixtures(ctx)
	if err != nil {
		t.Fatal("ListLocalFixtures failed: ", err)
	}

	want := []*protocol.ResolvedEntity{
		makeResolvedEntityForFixture("bundle1", 1, "fixt.LocalA", "fixt.RemoteA", "fixt.RemoteA"),
		makeResolvedEntityForFixture("bundle1", 1, "fixt.LocalB", "fixt.LocalA", "fixt.RemoteA"),
		makeResolvedEntityForFixture("bundle2", 1, "fixt.LocalA", "fixt.LocalB", "fixt.RemoteB"),
		makeResolvedEntityForFixture("bundle2", 1, "fixt.LocalB", "fixt.RemoteB", "fixt.RemoteB"),
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("Unexpected list of tests (-got +want):\n%v", diff)
	}
}

func TestDriver_ListRemoteFixtures(t *gotesting.T) {
	ctx, drv, _ := newDriverForListingTests(t)

	got, err := drv.ListRemoteFixtures(ctx)
	if err != nil {
		t.Fatal("ListRemoteFixtures failed: ", err)
	}

	want := []*protocol.ResolvedEntity{
		makeResolvedEntityForFixture("bundle1", 0, "fixt.RemoteA", "fixt.RemoteB", ""),
		makeResolvedEntityForFixture("bundle1", 0, "fixt.RemoteB", "", ""),
		makeResolvedEntityForFixture("bundle2", 0, "fixt.RemoteA", "", ""),
		makeResolvedEntityForFixture("bundle2", 0, "fixt.RemoteB", "fixt.RemoteA", ""),
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("Unexpected list of tests (-got +want):\n%v", diff)
	}
}
