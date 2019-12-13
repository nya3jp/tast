// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package command

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DurationFlag implements flag.Value to save a user-supplied integer time duration
// with fixed units to a time.Duration.
type DurationFlag struct {
	units time.Duration
	dst   *time.Duration
}

// NewDurationFlag returns a DurationFlag that will save a duration with the supplied units to dst.
func NewDurationFlag(units time.Duration, dst *time.Duration, def time.Duration) *DurationFlag {
	*dst = def
	return &DurationFlag{units, dst}
}

// Set sets the flag value.
func (f *DurationFlag) Set(v string) error {
	num, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return err
	}
	*f.dst = time.Duration(num) * f.units
	return nil
}

func (f *DurationFlag) String() string { return "" }

// EnumFlag implements flag.Value to map a user-supplied string value to an enum value.
type EnumFlag struct {
	valid  map[string]int     // map from user-supplied string value to int value
	assign EnumFlagAssignFunc // used to assign int value to dest
	def    string             // default value
}

// EnumFlagAssignFunc is used by EnumFlag to assign an enum value to a target variable.
type EnumFlagAssignFunc func(val int)

// NewEnumFlag returns an EnumFlag using the supplied map of valid values and assignment function.
// def contains a default value to assign when the flag is unspecified.
func NewEnumFlag(valid map[string]int, assign EnumFlagAssignFunc, def string) *EnumFlag {
	f := EnumFlag{valid, assign, def}
	if err := f.Set(def); err != nil {
		panic(err)
	}
	return &f
}

// Default returns the default value used if the flag is unset.
func (f *EnumFlag) Default() string { return f.def }

// QuotedValues returns a comma-separated list of quoted values the user can supply.
func (f *EnumFlag) QuotedValues() string {
	var qn []string
	for n := range f.valid {
		qn = append(qn, fmt.Sprintf("%q", n))
	}
	sort.Strings(qn)
	return strings.Join(qn, ", ")
}

func (f *EnumFlag) String() string { return "" }

// Set sets the flag value.
func (f *EnumFlag) Set(v string) error {
	ev, ok := f.valid[v]
	if !ok {
		return fmt.Errorf("must be in %s", f.QuotedValues())
	}
	f.assign(ev)
	return nil
}

// ListFlag implements flag.Value to split a user-supplied string with a custom delimiter
// into a slice of strings.
type ListFlag struct {
	sep    string             // value separator, e.g. ","
	assign ListFlagAssignFunc // used to assign slice value to dest
	def    []string           // default value, e.g. []string{"foo", "bar"}
}

// ListFlagAssignFunc is called by ListFlag to assign a slice to a target variable.
type ListFlagAssignFunc func(vals []string)

// NewListFlag returns a ListFlag using the supplied separator and assignment function.
// def contains a default value to assign when the flag is unspecified.
func NewListFlag(sep string, assign ListFlagAssignFunc, def []string) *ListFlag {
	f := ListFlag{sep, assign, def}
	f.Set(f.Default())
	return &f
}

// Default returns the default value used if the flag is unset.
func (f *ListFlag) Default() string { return strings.Join(f.def, f.sep) }

func (f *ListFlag) String() string { return "" }

// Set sets the flag value.
func (f *ListFlag) Set(v string) error {
	vals := strings.Split(v, f.sep)
	if len(vals) == 1 && vals[0] == "" {
		vals = nil
	}
	f.assign(vals)
	return nil
}

// RepeatedFlag implements flag.Value around an assignment function that is executed each
// time the flag is supplied.
type RepeatedFlag func(v string) error

// Default returns the default value used if the flag is unset.
func (f *RepeatedFlag) Default() string { return "" }

func (f *RepeatedFlag) String() string { return "" }

// Set sets the flag value.
func (f *RepeatedFlag) Set(v string) error { return (*f)(v) }
