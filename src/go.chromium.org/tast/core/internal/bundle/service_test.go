// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle_test

import (
	"context"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"go.chromium.org/tast/core/internal/bundle/bundletest"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/testing"

	frameworkprotocol "go.chromium.org/tast/core/framework/protocol"
)

func TestTestServiceListEntities(t *gotesting.T) {
	t1 := &testing.TestInstance{Name: "pkg.Test1"}
	t2 := &testing.TestInstance{Name: "pkg.Test2"}
	f1 := &testing.FixtureInstance{Name: "fixt1"}
	f2 := &testing.FixtureInstance{Name: "fixt2"}

	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(t1)
	reg.AddTestInstance(t2)
	reg.AddFixtureInstance(f1)
	reg.AddFixtureInstance(f2)

	env := bundletest.SetUp(t, bundletest.WithRemoteBundles(reg))
	cl := protocol.NewTestServiceClient(env.DialRemoteBundle(context.Background(), t, reg.Name()))

	got, err := cl.ListEntities(context.Background(), &protocol.ListEntitiesRequest{})
	if err != nil {
		t.Fatalf("ListEntities failed: %v", err)
	}

	want := &protocol.ListEntitiesResponse{
		Entities: []*protocol.ResolvedEntity{
			{Entity: f1.EntityProto()},
			{Entity: f2.EntityProto()},
			{Entity: t1.EntityProto()},
			{Entity: t2.EntityProto()},
		},
	}
	sorter := func(a, b *protocol.ResolvedEntity) bool {
		return a.GetEntity().GetName() < b.GetEntity().GetName()
	}
	if diff := cmp.Diff(got, want, protocmp.Transform(), protocmp.SortRepeated(sorter)); diff != "" {
		t.Errorf("ListEntitiesResponse mismatch (-got +want):\n%s", diff)
	}
}

func TestTestServiceGlobalRuntimeVars(t *gotesting.T) {

	reg := testing.NewRegistry("bundle")
	var1 := testing.NewVarString("var1", "", "description")
	reg.AddVar(var1)
	var2 := testing.NewVarString("var2", "", "description")
	reg.AddVar(var2)

	env := bundletest.SetUp(t, bundletest.WithRemoteBundles(reg))
	cl := protocol.NewTestServiceClient(env.DialRemoteBundle(context.Background(), t, reg.Name()))

	got, err := cl.GlobalRuntimeVars(context.Background(), &protocol.GlobalRuntimeVarsRequest{})
	if err != nil {
		t.Fatalf("GlobalRuntimeVars failed: %v", err)
	}

	want := &protocol.GlobalRuntimeVarsResponse{
		Vars: []*protocol.GlobalRuntimeVar{
			{Name: "var1"},
			{Name: "var2"},
		},
	}
	sorter := func(a, b *protocol.GlobalRuntimeVar) bool {
		return a.GetName() < b.GetName()
	}
	if diff := cmp.Diff(got, want, protocmp.Transform(), protocmp.SortRepeated(sorter)); diff != "" {
		t.Errorf("GlobalRuntimeVars mismatch (-got +want):\n%s", diff)
	}
}

func TestTestServerListEntitiesTestSkips(t *gotesting.T) {
	features := &protocol.Features{
		CheckDeps: true,
		Dut: &frameworkprotocol.DUTFeatures{
			Software: &frameworkprotocol.SoftwareFeatures{
				Available:   []string{"dep1"},
				Unavailable: []string{"dep2"},
			},
		},
	}
	t1 := &testing.TestInstance{Name: "pkg.Test1", SoftwareDeps: map[string][]string{"": []string{"dep1"}}}
	t2 := &testing.TestInstance{Name: "pkg.Test2", SoftwareDeps: map[string][]string{"": []string{"dep2"}}}
	t3 := &testing.TestInstance{Name: "pkg.Test3", SoftwareDeps: map[string][]string{"": []string{"dep3"}}}

	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(t1)
	reg.AddTestInstance(t2)
	reg.AddTestInstance(t3)

	env := bundletest.SetUp(t, bundletest.WithRemoteBundles(reg))
	cl := protocol.NewTestServiceClient(env.DialRemoteBundle(context.Background(), t, reg.Name()))

	// Call ListEntities.
	got, err := cl.ListEntities(context.Background(), &protocol.ListEntitiesRequest{Features: features})
	if err != nil {
		t.Fatalf("ListEntities failed: %v", err)
	}

	want := &protocol.ListEntitiesResponse{
		Entities: []*protocol.ResolvedEntity{
			// Test1 is not skipped.
			{Entity: t1.EntityProto()},
			// Test2 is skipped due to unavailable dep2.
			{Entity: t2.EntityProto(), Skip: &protocol.Skip{Reasons: []string{"missing SoftwareDeps: dep2"}}},
			// Test3 is not skipped due to a dependency check failure.
			// It fails later when we actually attempt to run it.
			{Entity: t3.EntityProto()},
		},
	}
	sorter := func(a, b *protocol.ResolvedEntity) bool {
		return a.GetEntity().GetName() < b.GetEntity().GetName()
	}
	if diff := cmp.Diff(got, want, protocmp.Transform(), protocmp.SortRepeated(sorter)); diff != "" {
		t.Errorf("ListEntitiesResponse mismatch (-got +want):\n%s", diff)
	}
}

func TestTestServerListEntitiesStartFixtureNames(t *gotesting.T) {
	t1 := &testing.TestInstance{Name: "test.Default"}
	t2 := &testing.TestInstance{Name: "test.DirectExternal", Fixture: "external"}
	t3 := &testing.TestInstance{Name: "test.IndirectExternal", Fixture: "fixt.IndirectExternal"}
	f1 := &testing.FixtureInstance{Name: "fixt.Default"}
	f2 := &testing.FixtureInstance{Name: "fixt.DirectExternal", Parent: "external"}
	f3 := &testing.FixtureInstance{Name: "fixt.IndirectExternal", Parent: "fixt.DirectExternal"}

	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(t1)
	reg.AddTestInstance(t2)
	reg.AddTestInstance(t3)
	reg.AddFixtureInstance(f1)
	reg.AddFixtureInstance(f2)
	reg.AddFixtureInstance(f3)

	env := bundletest.SetUp(t, bundletest.WithRemoteBundles(reg))
	cl := protocol.NewTestServiceClient(env.DialRemoteBundle(context.Background(), t, reg.Name()))

	// Call ListEntities.
	got, err := cl.ListEntities(context.Background(), &protocol.ListEntitiesRequest{})
	if err != nil {
		t.Fatalf("ListEntities failed: %v", err)
	}

	want := &protocol.ListEntitiesResponse{
		Entities: []*protocol.ResolvedEntity{
			{Entity: f1.EntityProto()},
			{Entity: f2.EntityProto(), StartFixtureName: "external"},
			{Entity: f3.EntityProto(), StartFixtureName: "external"},
			{Entity: t1.EntityProto()},
			{Entity: t2.EntityProto(), StartFixtureName: "external"},
			{Entity: t3.EntityProto(), StartFixtureName: "external"},
		},
	}
	sorter := func(a, b *protocol.ResolvedEntity) bool {
		return a.GetEntity().GetName() < b.GetEntity().GetName()
	}
	if diff := cmp.Diff(got, want, protocmp.Transform(), protocmp.SortRepeated(sorter)); diff != "" {
		t.Errorf("ListEntitiesResponse mismatch (-got +want):\n%s", diff)
	}
}

func TestTestServerListEntitiesRecursive(t *gotesting.T) {
	t1 := &testing.TestInstance{Name: "pkg.Test1"}
	t2 := &testing.TestInstance{Name: "pkg.Test2"}
	f1 := &testing.FixtureInstance{Name: "fixt1"}
	f2 := &testing.FixtureInstance{Name: "fixt2"}

	reg := testing.NewRegistry("cros")
	reg.AddTestInstance(t1)
	reg.AddFixtureInstance(f1)

	targetReg := testing.NewRegistry("cros")
	targetReg.AddTestInstance(t2)
	targetReg.AddFixtureInstance(f2)

	env := bundletest.SetUp(t,
		bundletest.WithRemoteBundles(reg),
		bundletest.WithLocalBundles(targetReg),
	)
	cl := protocol.NewTestServiceClient(env.DialRemoteBundle(context.Background(), t, reg.Name()))

	got, err := cl.ListEntities(context.Background(), &protocol.ListEntitiesRequest{
		Recursive: true,
	})
	if err != nil {
		t.Fatalf("ListEntities failed: %v", err)
	}

	want := &protocol.ListEntitiesResponse{
		Entities: []*protocol.ResolvedEntity{
			{Entity: f1.EntityProto()},
			{Entity: f2.EntityProto(), Hops: 1},
			{Entity: t1.EntityProto()},
			{Entity: t2.EntityProto(), Hops: 1},
		},
	}
	sorter := func(a, b *protocol.ResolvedEntity) bool {
		return a.GetEntity().GetName() < b.GetEntity().GetName()
	}
	if diff := cmp.Diff(got, want, protocmp.Transform(), protocmp.SortRepeated(sorter)); diff != "" {
		t.Errorf("ListEntitiesResponse mismatch (-got +want):\n%s", diff)
	}
}
