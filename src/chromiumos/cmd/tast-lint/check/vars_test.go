// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestVarFile(t *testing.T) {
	for _, tc := range []struct {
		filename     string
		data         string
		wantIssueStr []string
	}{
		{
			filename: "foo.Bar.yaml",
			data: `foo.Bar.baz: 123
foo.Bar.colon: "1:1"
foo.Bar.baz.1: 123
foo.Bar.: 123
foo.Foo.baz: 123
#comment: should be ignored`,
			wantIssueStr: []string{
				`foo.Bar.yaml:3:1: variable name "foo.Bar.baz.1"; should have the form of "foo.Bar.X" or "foo.X" where X matches [a-zA-Z]\w*`,
				`foo.Bar.yaml:4:1: variable name "foo.Bar."; should have the form of "foo.Bar.X" or "foo.X" where X matches [a-zA-Z]\w*`,
				`foo.Bar.yaml:5:1: variable "foo.Foo.baz" should be defined in file "foo.Foo.yaml"`,
			},
		},
		{
			filename: "foo.yaml",
			data: `foo.baz: 123
foo.baz.1: 123
foo.Foo.baz: 123`,
			wantIssueStr: []string{
				`foo.yaml:2:1: variable name "foo.baz.1"; should have the form of "foo.Bar.X" or "foo.X" where X matches [a-zA-Z]\w*`,
				`foo.yaml:3:1: variable "foo.Foo.baz" should be defined in file "foo.Foo.yaml"`,
			},
		},
	} {
		issues := VarFile(tc.filename, []byte(tc.data))
		var got []string
		for _, i := range issues {
			got = append(got, i.String())
		}
		if diff := cmp.Diff(got, tc.wantIssueStr); diff != "" {
			t.Errorf("Test for %s failed: (-got; +want):\n%v", tc.filename, diff)
		}
	}
}
