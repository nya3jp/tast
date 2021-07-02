// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	gotesting "testing"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/cmd/tast/internal/run/runtest"
	"chromiumos/tast/cmd/tast/internal/run/target"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testing"
)

func TestListLocalTests(t *gotesting.T) {
	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(&testing.TestInstance{
		Name: "pkg.Test",
		Desc: "This is a test",
		Attr: []string{"attr1", "attr2"},
	})
	reg.AddTestInstance(&testing.TestInstance{
		Name: "pkg.AnotherTest",
		Desc: "Another test",
	})

	env := runtest.SetUp(t, runtest.WithLocalBundles(reg))
	ctx := env.Context()
	cfg := env.Config()

	cc := target.NewConnCache(cfg, cfg.Target)
	defer cc.Close(ctx)

	conn, err := cc.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}

	got, err := ListLocalTests(ctx, cfg, nil, conn.SSHConn())
	if err != nil {
		t.Fatal("Failed to list local tests: ", err)
	}

	want := []*protocol.ResolvedEntity{
		{
			Entity: &protocol.Entity{
				Name:         "pkg.Test",
				Description:  "This is a test",
				Attributes:   []string{"attr1", "attr2"},
				Dependencies: &protocol.EntityDependencies{},
				Contacts:     &protocol.EntityContacts{},
				LegacyData: &protocol.EntityLegacyData{
					Timeout: ptypes.DurationProto(0),
					Bundle:  "bundle",
				},
			},
			Hops: 1,
		},
		{
			Entity: &protocol.Entity{
				Name:         "pkg.AnotherTest",
				Description:  "Another test",
				Dependencies: &protocol.EntityDependencies{},
				Contacts:     &protocol.EntityContacts{},
				LegacyData: &protocol.EntityLegacyData{
					Timeout: ptypes.DurationProto(0),
					Bundle:  "bundle",
				},
			},
			Hops: 1,
		},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("Unexpected list of local tests (-got +want):\n%v", diff)
	}
}

func TestListRemoteTests(t *gotesting.T) {
	reg := testing.NewRegistry("bundle")
	reg.AddTestInstance(&testing.TestInstance{
		Name: "pkg.Test1",
		Desc: "First description",
		Attr: []string{"attr1", "attr2"},
		Pkg:  "pkg",
	})
	reg.AddTestInstance(&testing.TestInstance{
		Name: "pkg2.Test2",
		Desc: "Second description",
		Attr: []string{"attr3"},
		Pkg:  "pkg2",
	})

	env := runtest.SetUp(t, runtest.WithRemoteBundles(reg))
	ctx := env.Context()
	cfg := env.Config()

	got, err := listRemoteTests(ctx, cfg, nil)
	if err != nil {
		t.Error("Failed to list remote tests: ", err)
	}

	want := []*protocol.ResolvedEntity{
		{
			Entity: &protocol.Entity{
				Name:         "pkg.Test1",
				Description:  "First description",
				Attributes:   []string{"attr1", "attr2"},
				Package:      "pkg",
				Dependencies: &protocol.EntityDependencies{},
				Contacts:     &protocol.EntityContacts{},
				LegacyData: &protocol.EntityLegacyData{
					Timeout: ptypes.DurationProto(0),
					Bundle:  "bundle",
				},
			},
			Hops: 0,
		},
		{
			Entity: &protocol.Entity{
				Name:         "pkg2.Test2",
				Description:  "Second description",
				Attributes:   []string{"attr3"},
				Package:      "pkg2",
				Dependencies: &protocol.EntityDependencies{},
				Contacts:     &protocol.EntityContacts{},
				LegacyData: &protocol.EntityLegacyData{
					Timeout: ptypes.DurationProto(0),
					Bundle:  "bundle",
				},
			},
			Hops: 0,
		},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("Unexpected list of remote tests (-got +want):\n%v", diff)
	}
}
