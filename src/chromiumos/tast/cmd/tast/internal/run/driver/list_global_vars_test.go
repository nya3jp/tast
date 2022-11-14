// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package driver_test

import (
	"context"
	gotesting "testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/driver"
	"chromiumos/tast/cmd/tast/internal/run/runtest"
	"chromiumos/tast/internal/testing"
)

func newDriverForGlobalRuntimeVars(t *gotesting.T) (context.Context, *driver.Driver) {
	local1 := testing.NewRegistry("bundle1")
	var1 := testing.NewVarString("var1", "", "description")
	local1.AddVar(var1)

	remote1 := testing.NewRegistry("bundle1")
	var2 := testing.NewVarString("var2", "", "description")
	remote1.AddVar(var2)

	local2 := testing.NewRegistry("bundle2")
	var3 := testing.NewVarString("var3", "", "description")
	local2.AddVar(var3)

	remote2 := testing.NewRegistry("bundle2")
	var4 := testing.NewVarString("var4", "", "description")
	remote2.AddVar(var4)

	env := runtest.SetUp(t, runtest.WithLocalBundles(local1, local2), runtest.WithRemoteBundles(remote1, remote2))
	ctx := env.Context()
	cfg := env.Config(func(cfg *config.MutableConfig) {
		cfg.Patterns = []string{"(yes)"}
	})

	drv, err := driver.New(ctx, cfg, cfg.Target(), "")
	if err != nil {
		t.Fatalf("driver.New failed: %v", err)
	}
	t.Cleanup(func() { drv.Close(ctx) })

	return ctx, drv
}

func TestDriver_GlobalRuntimeVars(t *gotesting.T) {
	ctx, drv := newDriverForGlobalRuntimeVars(t)

	got, err := drv.GlobalRuntimeVars(ctx)
	if err != nil {
		t.Fatal("GlobalRuntimeVars failed: ", err)
	}

	want := []string{
		"var1", "var2", "var3", "var4",
	}
	if diff := cmp.Diff(got, want, protocmp.Transform()); diff != "" {
		t.Errorf("Unexpected list of Global runtime Vars (-got +want):\n%v", diff)
	}
}
