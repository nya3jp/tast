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
	"os"
	"reflect"
	gotesting "testing"

	"github.com/google/subcommands"

	"chromiumos/tast/internal/run/resultsjson"
	"chromiumos/tast/testutil"
)

// executeListCmd creates a listCmd and executes it using the supplied args and wrapper.
func executeListCmd(t *gotesting.T, stdout io.Writer, args []string, wrapper *stubRunWrapper) subcommands.ExitStatus {
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
	return cmd.Execute(context.Background(), flags)
}

func TestListTests(t *gotesting.T) {
	test1 := resultsjson.Test{Name: "pkg.Test1", Desc: "First description", Attr: []string{"attr1"}}
	test2 := resultsjson.Test{Name: "pkg.Test2", Desc: "Second description"}
	wrapper := stubRunWrapper{
		runRes: []*resultsjson.Result{{Test: test1}, {Test: test2}},
	}

	// Verify that the default one-test-per-line mode works.
	stdout := bytes.Buffer{}
	args := []string{"root@example.net"}
	if status := executeListCmd(t, &stdout, args, &wrapper); status != subcommands.ExitSuccess {
		t.Fatalf("listCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitSuccess)
	}
	if exp := fmt.Sprintf("%s\n%s\n", test1.Name, test2.Name); stdout.String() != exp {
		t.Errorf("listCmd.Execute(%v) printed %q; want %q", args, stdout.String(), exp)
	}

	// Verify that full test objects are written as JSON when -json is supplied.
	stdout.Reset()
	args = append([]string{"-json"}, args...)
	if status := executeListCmd(t, &stdout, args, &wrapper); status != subcommands.ExitSuccess {
		t.Fatalf("listCmd.Execute(%v) returned status %v; want %v", args, status, subcommands.ExitSuccess)
	}
	var act []resultsjson.Test
	if err := json.Unmarshal(stdout.Bytes(), &act); err != nil {
		t.Errorf("Failed to unmarshal output from listCmd.Execute(%v): %v", args, err)
	}
	if exp := []resultsjson.Test{test1, test2}; !reflect.DeepEqual(exp, act) {
		t.Errorf("listCmd.Execute(%v) printed %+v; want %+v", args, act, exp)
	}
}
