// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	gotesting "testing"

	"chromiumos/tast/common/testing"
)

func MyFunc(*testing.State) {}

func TestListDataFiles(t *gotesting.T) {
	testing.ClearForTesting()
	defer testing.ClearForTesting()

	testing.AddTest(&testing.Test{
		Name: "foo.Test1",
		Func: MyFunc,
		Data: []string{"1", "file_{arch}"},
	})
	testing.AddTest(&testing.Test{
		Name: "foo.Test2",
		Func: MyFunc,
		Data: []string{"1", "2"},
	})

	tests := testing.GlobalRegistry().AllTests()
	const arch = "myarch"
	b := bytes.Buffer{}
	if err := listDataFiles(&b, tests, arch); err != nil {
		t.Fatalf("listDataFiles(b, %v, %v) failed: %v", tests, arch, err)
	}

	exp := []string{
		filepath.Join(tests[0].DataDir(), testing.TestDataPathForArch("1", arch)),
		filepath.Join(tests[0].DataDir(), testing.TestDataPathForArch("file_{arch}", arch)),
		filepath.Join(tests[1].DataDir(), testing.TestDataPathForArch("2", arch)),
	}
	act := make([]string, 0)
	if err := json.Unmarshal(b.Bytes(), &act); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(exp, act) {
		t.Errorf("listDataFiles(b, %v, %v) wrote %v; want %v", tests, arch, act, exp)
	}
}
