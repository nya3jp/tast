// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package command_test

import (
	"flag"
	"fmt"
	"io/ioutil"
	"reflect"
	"strconv"
	"testing"
	"time"

	"chromiumos/tast/internal/command"
)

func TestDurationFlag(t *testing.T) {
	for _, tc := range []struct {
		units time.Duration // units for flag
		args  []string      // args to parse
		def   time.Duration // default value for flag
		exp   time.Duration // expected value
	}{
		{time.Second, []string{}, 0, 0},
		{time.Second, []string{}, 10 * time.Second, 10 * time.Second},
		{time.Second, []string{"-flag=5"}, 0, 5 * time.Second},
		{time.Minute, []string{"-flag=2"}, 0, 2 * time.Minute},
		{time.Millisecond, []string{"-flag=200"}, 0, 200 * time.Millisecond},
	} {
		var d time.Duration
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		fs.SetOutput(ioutil.Discard)
		fs.Var(command.NewDurationFlag(tc.units, &d, tc.def), "flag", "usage")

		if err := fs.Parse(tc.args); err != nil {
			t.Errorf("%v produced error: %v", tc.args, err)
		} else if d != tc.exp {
			t.Errorf("%v resulted in %v; want %v", tc.args, d, tc.exp)
		}
	}
}

func ExampleDurationFlag() {
	var dest time.Duration
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	flags.Var(command.NewDurationFlag(time.Second, &dest, 5*time.Second), "flag", "usage")

	// When the flag isn't supplied, the default is used.
	flags.Parse([]string{})
	fmt.Println("no flag:", dest)

	// When the flag is supplied, it's interpreted as an integer duration using the supplied units.
	flags.Parse([]string{"-flag=10"})
	fmt.Println("flag:", dest)

	// Output:
	// no flag: 5s
	// flag: 10s
}

func TestEnumFlag(t *testing.T) {
	type testEnum int
	const (
		testVal0 testEnum = iota
		testVal1
		testVal2
	)

	for _, tc := range []struct {
		args   []string // args to parse
		def    string   // default value for flag
		exp    testEnum // expected value
		expErr bool     // if true, error is expected
	}{
		{[]string{}, "val0", testVal0, false},
		{[]string{"-flag=val0"}, "val0", testVal0, false},
		{[]string{"-flag=val1"}, "val0", testVal1, false},
		{[]string{"-flag=val2"}, "val0", testVal2, false},
		{[]string{"-flag=bogus"}, "val0", testVal0, true},
		{[]string{"-flag"}, "val0", testVal0, true},
	} {
		valid := map[string]int{"val0": int(testVal0), "val1": int(testVal1), "val2": int(testVal2)}
		val := testEnum(-1)
		f := func(v int) { val = testEnum(v) }
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		fs.SetOutput(ioutil.Discard)
		fs.Var(command.NewEnumFlag(valid, f, tc.def), "flag", "usage")

		if err := fs.Parse(tc.args); err != nil && !tc.expErr {
			t.Errorf("%v produced error: %v", tc.args, err)
		} else if err == nil && tc.expErr {
			t.Errorf("%v didn't produce expected error", tc.args)
		} else if val != tc.exp {
			t.Errorf("%v resulted in %v; want %v", tc.args, val, tc.exp)
		}
	}
}

func ExampleEnumFlag() {
	type enum int
	const (
		foo enum = 1
		bar      = 2
	)

	var dest enum
	valid := map[string]int{"foo": int(foo), "bar": int(bar)}
	assign := func(v int) { dest = enum(v) }
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	flags.Var(command.NewEnumFlag(valid, assign, "foo"), "flag", "usage")

	// When the flag isn't supplied, the default is used.
	flags.Parse([]string{})
	fmt.Println("no flag:", dest)

	// When a value is supplied, it's converted to the corresponding enum value.
	flags.Parse([]string{"-flag=bar"})
	fmt.Println("flag:", dest)

	// Output:
	// no flag: 1
	// flag: 2
}

func TestListFlag(t *testing.T) {
	for _, tc := range []struct {
		sep  string   // separator to use
		args []string // args to parse
		def  []string // default value for flag
		exp  []string // expected values
	}{
		{",", []string{}, nil, nil},
		{",", []string{}, []string{"foo", "bar"}, []string{"foo", "bar"}},
		{",", []string{"-flag=foo"}, nil, []string{"foo"}},
		{",", []string{"-flag=foo,bar"}, nil, []string{"foo", "bar"}},
		{",", []string{"-flag=foo,bar"}, []string{"default"}, []string{"foo", "bar"}},
		{" ", []string{"-flag=foo bar"}, []string{"default"}, []string{"foo", "bar"}},
		{":", []string{"-flag=foo:bar"}, []string{"default"}, []string{"foo", "bar"}},
	} {
		var vals []string
		f := func(v []string) { vals = v }
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		fs.SetOutput(ioutil.Discard)
		fs.Var(command.NewListFlag(tc.sep, f, tc.def), "flag", "usage")

		if err := fs.Parse(tc.args); err != nil {
			t.Errorf("%v produced error: %v", tc.args, err)
		} else if !reflect.DeepEqual(vals, tc.exp) {
			t.Errorf("%v resulted in %v; want %v", tc.args, vals, tc.exp)
		}
	}
}

func ExampleListFlag() {
	var dest []string
	assign := func(v []string) { dest = v }
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	flags.Var(command.NewListFlag(",", assign, []string{"a", "b"}), "flag", "usage")

	// When the flag isn't supplied, the default is used.
	flags.Parse([]string{})
	fmt.Println("no flag:", dest)

	// When the flag is supplied, its value is split into a slice.
	flags.Parse([]string{"-flag=c,d,e"})
	fmt.Println("flag:", dest)

	// Output:
	// no flag: [a b]
	// flag: [c d e]
}

func ExampleRepeatedFlag() {
	var dest []int
	rf := command.RepeatedFlag(func(v string) error {
		i, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		dest = append(dest, i)
		return nil
	})
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	flags.Var(&rf, "flag", "usage")

	// When the flag isn't supplied, the slice is unchanged.
	flags.Parse([]string{})
	fmt.Println("no flag:", dest)

	// The function is called each time the flag is provided.
	flags.Parse([]string{"-flag=1", "-flag=2"})
	fmt.Println("flag:", dest)

	// Output:
	// no flag: []
	// flag: [1 2]
}
