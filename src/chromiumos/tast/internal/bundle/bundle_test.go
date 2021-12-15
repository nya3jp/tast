// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	gotesting "testing"

	"chromiumos/tast/internal/bundle/legacyjson"
	"chromiumos/tast/internal/testing"
)

func TestDumpTestsJSON(t *gotesting.T) {
	reg := testing.NewRegistry("bundle")

	f := func(context.Context, *testing.State) {}
	tests := []*testing.TestInstance{
		{Name: "pkg.Test", Func: f},
		{Name: "pkg.Test2", Func: f},
	}

	for _, test := range tests {
		reg.AddTestInstance(test)
	}

	var infos []*legacyjson.EntityWithRunnabilityInfo
	for _, test := range tests {
		infos = append(infos, &legacyjson.EntityWithRunnabilityInfo{
			EntityInfo: *legacyjson.MustEntityInfoFromProto(test.EntityProto()),
		})
	}

	var exp bytes.Buffer
	if err := json.NewEncoder(&exp).Encode(infos); err != nil {
		t.Fatal(err)
	}

	clArgs := []string{"-dumptests"}
	stdout := &bytes.Buffer{}
	if status := run(context.Background(), clArgs, &bytes.Buffer{}, stdout, &bytes.Buffer{}, NewStaticConfig(reg, 0, Delegate{})); status != statusSuccess {
		t.Fatalf("run(%v) returned status %v; want %v", clArgs, status, statusSuccess)
	}
	if stdout.String() != exp.String() {
		t.Errorf("run(%v) wrote %q; want %q", clArgs, stdout.String(), exp.String())
	}
}

func TestDumpTestsJSON_RegistrationErrors(t *gotesting.T) {
	reg := testing.NewRegistry("bundle")
	const name = "cat.MyTest"
	reg.AddTestInstance(&testing.TestInstance{Name: name, Func: testFunc})

	// Adding a test with same name should generate an error.
	reg.AddTestInstance(&testing.TestInstance{Name: name, Func: testFunc})

	clArgs := []string{"-dumptests"}
	if status := run(context.Background(), clArgs, &bytes.Buffer{}, ioutil.Discard, ioutil.Discard, NewStaticConfig(reg, 0, Delegate{})); status != statusBadTests {
		t.Errorf("run() with bad test returned status %v; want %v", status, statusBadTests)
	}
}
