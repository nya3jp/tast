// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package command

import (
	"fmt"
	"sort"
	"strings"
)

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

func (f *EnumFlag) Set(v string) error {
	ev, ok := f.valid[v]
	if !ok {
		return fmt.Errorf("must be in %s", f.QuotedValues())
	}
	f.assign(ev)
	return nil
}
