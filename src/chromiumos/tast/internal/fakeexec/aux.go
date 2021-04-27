// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fakeexec

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
)

const (
	// auxMainNameEnv is the name of an environment variable that specifies
	// a name of an auxiliary main function to run.
	auxMainNameEnv = "AUX_MAIN_NAME"

	// auxMainValueEnv is the name of an environment variable that carries
	// an extra value passed to an auxiliary main function.
	auxMainValueEnv = "AUX_MAIN_VALUE"
)

// AuxMain represents a auxiliary main function.
type AuxMain struct {
	name string
}

// Params creates AuxMainParams that contains information necessary to execute
// the auxiliary main function.
// v should be an arbitrary JSON-serializable value. It is passed to the
// auxiliary main function.
func (a *AuxMain) Params(v interface{}) (*AuxMainParams, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	p, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &AuxMainParams{
		executable: exe,
		name:       a.name,
		param:      string(p),
	}, nil
}

// AuxMainParams contains information necessary to execute an auxiliary main
// function.
type AuxMainParams struct {
	executable string
	name       string
	param      string
}

// Executable returns a path to the current executable. It is similar to
// os.Executable, but it is precomputed and never fails.
func (a *AuxMainParams) Executable() string {
	return a.executable
}

// Envs returns environment variables to be set to execute the auxiliary main
// function. Elements are in the form of "key=value" so that they can be
// appended to os/exec.Cmd.Env.
func (a *AuxMainParams) Envs() []string {
	return []string{
		fmt.Sprintf("%s=%s", auxMainNameEnv, a.name),
		fmt.Sprintf("%s=%s", auxMainValueEnv, a.param),
	}
}

// SetEnvs modifies the current process' environment variables with os.Setenv
// so that the auxiliary main function is called on executing the self
// executable as a subprocess.
//
// It returns a closure to restore the original environment variable. It panics
// if the environment variable is already set.
func (a *AuxMainParams) SetEnvs() (restore func()) {
	if val := os.Getenv(auxMainNameEnv); val != "" {
		panic(fmt.Sprintf("fakeexec.AuxMain.SetEnv: Environment variable %s already set to non-empty value %q", auxMainNameEnv, val))
	}
	os.Setenv(auxMainNameEnv, a.name)
	os.Setenv(auxMainValueEnv, a.param)
	return func() {
		os.Unsetenv(auxMainNameEnv)
		os.Unsetenv(auxMainValueEnv)
	}
}

var knownNames = map[string]struct{}{}

// NewAuxMain registers a new auxiliary main function.
//
// name identifies an auxiliary main function. It must be unique within the
// current executable; otherwise this function will panic.
//
// f must be a function having a signature func(T) where T is a JSON
// serializable type.
//
// NewAuxMain must be called in a top-level variable initialization like:
//
//   type fooParams struct { ... }
//
//   var fooMain = fakeexec.NewAuxMain("foo", func(p fooParams) {
//     // Another main function here...
//   })
//
// If the current process is executed for the auxiliary main, NewAuxMain
// immediately calls f and exits. Otherwise *AuxMain is returned, which you can
// use to start a subprocess running the auxiliary main.
//
//   p := fooMain.Params(fooParams{ ... })
//
//   cmd := exec.Command(p.Name())
//   cmd.Env = append(os.Environ(), p.Envs()...)
//
//   if err := cmd.Run(); err != nil { ... }
//
// Prefer Loopback if subprocesses don't need to call system calls. Loopback
// subprocesses virtually run within the current unit test process, which is
// usually more convenient than auxiliary main functions that run as separate
// processes.
func NewAuxMain(name string, f interface{}) *AuxMain {
	if _, found := knownNames[name]; found {
		panic(fmt.Sprintf("fakeexec.NewAuxMain: Multiple registrations for %q", name))
	}
	knownNames[name] = struct{}{}

	tf := reflect.TypeOf(f)
	if ni, no := tf.NumIn(), tf.NumOut(); ni != 1 || no != 0 {
		panic(fmt.Sprintf("fakeexec.NewAuxMain: f has wrong signature: must be func(T)"))
	}
	tp := tf.In(0)

	if os.Getenv(auxMainNameEnv) != name {
		return &AuxMain{name: name}
	}

	// Run the auxiliary main function.
	vp := reflect.New(tp)
	if err := json.Unmarshal([]byte(os.Getenv(auxMainValueEnv)), vp.Interface()); err != nil {
		panic(fmt.Sprintf("fakeexec.AuxMain: %s: failed to unmarshal parameter: %v", name, err))
	}
	reflect.ValueOf(f).Call([]reflect.Value{vp.Elem()})
	os.Exit(0)
	panic("unreachable")
}
