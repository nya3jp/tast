// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	gotesting "testing"

	"chromiumos/tast/testing"
)

func TestListDataFiles(t *gotesting.T) {
	reg := testing.NewRegistry()
	reg.DisableValidationForTesting()
	for _, test := range []*testing.Test{
		&testing.Test{Func: func(*testing.State) {}, Data: []string{"1"}},
		&testing.Test{Func: func(*testing.State) {}, Data: []string{"1", "2"}},
	} {
		if err := reg.AddTest(test); err != nil {
			t.Fatal(err)
		}
	}

	tests := reg.AllTests()
	b := bytes.Buffer{}
	if err := listDataFiles(&b, tests); err != nil {
		t.Fatalf("listDataFiles(b, %v) failed: %v", tests, err)
	}

	exp := []string{
		filepath.Join(tests[0].DataDir(), "1"),
		filepath.Join(tests[1].DataDir(), "2"),
	}
	act := make([]string, 0)
	if err := json.Unmarshal(b.Bytes(), &act); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(exp, act) {
		t.Errorf("listDataFiles(b, %v) wrote %v; want %v", tests, act, exp)
	}
}
