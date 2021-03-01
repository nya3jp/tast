// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/testutil"
)

func TestFindVarsFiles(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	if err := testutil.WriteFiles(td, map[string]string{
		"valid1.yaml":    "",
		"valid2.yaml":    "",
		"ignored.txt":    "",
		"ignored.json":   "",
		"dir/inner.yaml": "",
	}); err != nil {
		t.Fatal(err)
	}

	paths, err := findVarsFiles(td)
	if err != nil {
		t.Fatal("findVarsFiles failed: ", err)
	}

	exp := []string{
		filepath.Join(td, "dir/inner.yaml"),
		filepath.Join(td, "valid1.yaml"),
		filepath.Join(td, "valid2.yaml"),
	}
	if diff := cmp.Diff(paths, exp); diff != "" {
		t.Errorf("findVarsFiles returned unexpected paths (-got +want):\n%s", diff)
	}
}

func TestFindVarsFilesNotExist(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	if _, err := findVarsFiles(filepath.Join(td, "no_such_dir")); err != nil {
		t.Fatal("findVarsFiles failed: ", err)
	}
}

func TestReadVars(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	if err := testutil.WriteFiles(td, map[string]string{
		"empty.yaml":   "",
		"test.yaml":    "a: foo\nb: bar",
		"invalid.yaml": "123",
	}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name      string
		want      map[string]string
		wantError bool
	}{
		{
			name: "empty.yaml",
			want: map[string]string{},
		},
		{
			name: "test.yaml",
			want: map[string]string{"a": "foo", "b": "bar"},
		},
		{
			name:      "invalid.yaml",
			wantError: true,
		},
		{
			name:      "missing.yaml",
			wantError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vars, err := readVarsFile(filepath.Join(td, tc.name))
			if err != nil {
				if !tc.wantError {
					t.Fatal("readVarsFile failed: ", err)
				}
				return
			}
			if tc.wantError {
				t.Fatal("readVarsFile succeeded unexpectedly")
			}
			if diff := cmp.Diff(vars, tc.want); diff != "" {
				t.Fatalf("readVarsFile returned unexpected vars (-got +want):\n%v", diff)
			}
		})
	}
}

func TestMergeVars(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	for _, tc := range []struct {
		name      string
		vars      map[string]string
		newVars   map[string]string
		mode      mergeVarsMode
		want      map[string]string
		wantError bool
	}{
		{
			name: "empty_skipOnDuplicate",
			vars: map[string]string{},
			mode: skipOnDuplicate,
			want: map[string]string{},
		},
		{
			name: "empty_errorOnDuplicate",
			vars: map[string]string{},
			mode: errorOnDuplicate,
			want: map[string]string{},
		},
		{
			name: "old_values_only",
			vars: map[string]string{"a": "foo", "b": "bar"},
			mode: errorOnDuplicate,
			want: map[string]string{"a": "foo", "b": "bar"},
		},
		{
			name:    "new_values_only",
			vars:    map[string]string{},
			newVars: map[string]string{"a": "foo", "b": "bar"},
			mode:    errorOnDuplicate,
			want:    map[string]string{"a": "foo", "b": "bar"},
		},
		{
			name:    "merge",
			vars:    map[string]string{"a": "foo", "b": "bar"},
			newVars: map[string]string{"c": "baz"},
			mode:    skipOnDuplicate,
			want:    map[string]string{"a": "foo", "b": "bar", "c": "baz"},
		},
		{
			name:    "duplicate_skipOnDuplicate",
			vars:    map[string]string{"a": "foo", "b": "bar"},
			newVars: map[string]string{"a": "hi"},
			mode:    skipOnDuplicate,
			want:    map[string]string{"a": "foo", "b": "bar"},
		},
		{
			name:      "duplicate_errorOnDuplicate",
			vars:      map[string]string{"a": "foo", "b": "bar"},
			newVars:   map[string]string{"a": "hi"},
			mode:      errorOnDuplicate,
			wantError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := mergeVars(tc.vars, tc.newVars, tc.mode); err != nil {
				if !tc.wantError {
					t.Fatal("mergeVars failed: ", err)
				}
				return
			}
			if tc.wantError {
				t.Fatal("mergeVars succeeded unexpectedly")
			}
			if diff := cmp.Diff(tc.vars, tc.want); diff != "" {
				t.Fatalf("mergeVars returned unexpected vars (-got +want):\n%v", diff)
			}
		})
	}
}
