// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fakeexec_test

import (
	"os"
	"os/exec"
	"testing"

	"chromiumos/tast/internal/fakeexec"
)

type auxParam struct {
	Str string
	Int int
}

var auxValues = auxParam{
	Str: "hello",
	Int: 42,
}

var auxMain = fakeexec.NewAuxMain("fakeexec_test", func(v auxParam) {
	if v != auxValues {
		os.Exit(28)
	}
	os.Exit(42)
})

func TestAuxMainEnvs(t *testing.T) {
	p, err := auxMain.Params(auxValues)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(p.Executable())
	cmd.Env = append(os.Environ(), p.Envs()...)
	err = cmd.Run()
	if xerr, ok := err.(*exec.ExitError); !ok || xerr.ExitCode() != 42 {
		t.Errorf("Run: %v; want exit code 42", err)
	}
}

func TestAuxMainSetEnvs(t *testing.T) {
	p, err := auxMain.Params(auxValues)
	if err != nil {
		t.Fatal(err)
	}

	restore := p.SetEnvs()
	defer restore()

	err = exec.Command(p.Executable()).Run()
	if xerr, ok := err.(*exec.ExitError); !ok || xerr.ExitCode() != 42 {
		t.Errorf("Run: %v; want exit code 42", err)
	}
}
