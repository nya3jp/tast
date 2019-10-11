// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestVarsReadAndUpdateVars(t *testing.T) {
	for _, tc := range []struct {
		name string
		// files is a map from file name to its content.
		files     map[string]string
		vars      map[string]string
		want      map[string]string
		wantError bool
	}{
		{
			name:  "nothing",
			files: map[string]string{},
			want:  map[string]string{},
		},
		{
			name:  "one file",
			files: map[string]string{"test.yaml": "foo: 42\nbaz: qux"},
			want:  map[string]string{"foo": "42", "baz": "qux"},
		},
		{
			name:  "filter by extension",
			files: map[string]string{"test.txt": "foo: bar"},
			want:  map[string]string{},
		},
		{
			name:      "invalid",
			files:     map[string]string{"invalid.yaml": "123"},
			wantError: true,
		},
		{
			name: "merge",
			files: map[string]string{
				"a.yaml": "foo: bar",
				"b.yaml": "baz: qux",
			},
			want: map[string]string{
				"foo": "bar",
				"baz": "qux",
			},
		},
		{
			name: "duplicated",
			files: map[string]string{
				"a.yaml": "foo: bar",
				"b.yaml": "foo: baz",
			},
			wantError: true,
		},
		{
			name: "vars",
			vars: map[string]string{
				"foo": "bar",
			},
			files: map[string]string{
				"a.yaml": "foo: qux",
				"b.yaml": "baz: qux",
			},
			want: map[string]string{
				"foo": "bar",
				"baz": "qux",
			},
		},
	} {
		// Wrap a test with func for defer statements not to be stacked.
		func() {
			varDir, err := ioutil.TempDir("", "vars")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(varDir)
			for k, v := range tc.files {
				ioutil.WriteFile(filepath.Join(varDir, k), []byte(v), 0644)
			}
			// It's fine to modify tc.vars in readAndUpdateVars, as it has no later use.
			vars := tc.vars
			if vars == nil {
				vars = map[string]string{}
			}

			if err := readAndUpdateVars(varDir, vars); err != nil {
				if !tc.wantError {
					t.Errorf("Test %q failed; unexpected error: %v", tc.name, err)
				}
				return
			}
			if tc.wantError {
				t.Errorf("Test %q failed; unexpected success", tc.name)
				return
			}
			if diff := cmp.Diff(tc.want, vars); diff != "" {
				t.Errorf("Test %q failed; (-want +got):\n%v", tc.name, diff)
			}
		}()
	}
}
func TestVarsReadAndUpdateVarsNotExist(t *testing.T) {
	tmp, err := ioutil.TempDir("", "tmp")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	if err := readAndUpdateVars(filepath.Join(tmp, "nonexistent"), nil); !os.IsNotExist(err) {
		t.Error("os.IsNotExist was unexpectedly false: ", err)
	}
}
