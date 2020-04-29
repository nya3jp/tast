// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	gotesting "testing"

	"github.com/google/subcommands"

	"chromiumos/tast/cmd/tast/internal/logging"
	"chromiumos/tast/cmd/tast/internal/run"
	"chromiumos/tast/testing"
	"chromiumos/tast/testutil"
)

// executeListCmd creates a listCmd and executes it using the supplied args and wrapper.
func executeListCmd(t *gotesting.T, stdout io.Writer, args []string,
	wrapper *stubRunWrapper, lg logging.Logger) subcommands.ExitStatus {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	cmd := newListCmd(stdout, td)
	cmd.wrapper = wrapper
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	cmd.SetFlags(flags)
	if err := flags.Parse(args); err != nil {
		t.Fatal(err)
	}
	flags.Set("build", "false") // DeriveDefaults fails if -build=true and bundle dirs are missing
	return cmd.Execute(logging.NewContext(context.Background(), lg), flags)
}

func TestListTests(t *gotesting.T) {
	test1 := testing.TestInstance{Name: "pkg.Test1", Desc: "First description", Attr: []string{"attr1"}}
	test2 := testing.TestInstance{Name: "pkg.Test2", Desc: "Second description"}
	wrapper := stubRunWrapper{
		runRes: []run.TestResult{{TestInstance: test1}, {TestInstance: test2}},
	}

	// Verify that the default one-test-per-line mode works.
	stdout := bytes.Buffer{}
	args := []string{"root@example.net"}
	if status := executeListCmd(t, &stdout, args, &wrapper, logging.NewDiscard()); status != subcommands.ExitSuccess {
		t.Fatalf("listCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitSuccess)
	}
	if exp := fmt.Sprintf("%s\n%s\n", test1.Name, test2.Name); stdout.String() != exp {
		t.Errorf("listCmd.Execute(%v) printed %q; want %q", args, stdout.String(), exp)
	}

	// Verify that full test objects are written as JSON when -json is supplied.
	stdout.Reset()
	args = append([]string{"-json"}, args...)
	if status := executeListCmd(t, &stdout, args, &wrapper, logging.NewDiscard()); status != subcommands.ExitSuccess {
		t.Fatalf("listCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitSuccess)
	}
	var act []testing.TestInstance
	if err := json.Unmarshal(stdout.Bytes(), &act); err != nil {
		t.Errorf("Failed to unmarshal output from listCmd.Execute(%v): %v", args, err)
	}
	if exp := []testing.TestInstance{test1, test2}; !reflect.DeepEqual(exp, act) {
		t.Errorf("listCmd.Execute(%v) printed %+v; want %+v", args, act, exp)
	}
}

func TestListTestsReadFile(t *gotesting.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	// Writes tests to fn within td and returns the full path.
	writeTests := func(fn string, tests []*testing.TestInstance) string {
		b, err := json.Marshal(tests)
		if err != nil {
			t.Fatal("Failed to marshal tests: ", err)
		}
		p := filepath.Join(td, fn)
		if err := ioutil.WriteFile(p, b, 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	test1 := testing.TestInstance{Name: "pkg.Test1", Attr: []string{"a"}}
	test2 := testing.TestInstance{Name: "pkg.Test2", Attr: []string{"b"}}
	p1 := writeTests("1.json", []*testing.TestInstance{&test1, &test2})

	test3 := testing.TestInstance{Name: "pkg.Test3", Attr: []string{"b"}}
	test4 := testing.TestInstance{Name: "pkg.Test4", Attr: []string{"a"}}
	p2 := writeTests("2.json", []*testing.TestInstance{&test3, &test4})

	var stdout bytes.Buffer
	args := []string{"-json", "-readfile=" + p1, "-readfile=" + p2, "(b)"}
	if status := executeListCmd(t, &stdout, args, nil, logging.NewDiscard()); status != subcommands.ExitSuccess {
		t.Fatalf("listCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitSuccess)
	}
	var act []testing.TestInstance
	if err := json.Unmarshal(stdout.Bytes(), &act); err != nil {
		t.Errorf("Failed to unmarshal output from listCmd.Execute(%v): %v", args, err)
	} else if exp := []testing.TestInstance{test2, test3}; !reflect.DeepEqual(exp, act) {
		t.Errorf("listCmd.Execute(%v) printed %+v; want %+v", args, act, exp)
	}
}
