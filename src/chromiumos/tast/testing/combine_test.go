// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
	gotesting "testing"
)

func TestCombine(t *gotesting.T) {
	params := Combine(
		NewAxis("str", "a", "b", "c"),
		NewAxis("Int", 1, 2),
		NewAxis("b", true, false))
	expected := [][]interface{}{
		{"a", 1, true},
		{"a", 1, false},
		{"a", 2, true},
		{"a", 2, false},
		{"b", 1, true},
		{"b", 1, false},
		{"b", 2, true},
		{"b", 2, false},
		{"c", 1, true},
		{"c", 1, false},
		{"c", 2, true},
		{"c", 2, false},
	}
	for _, p := range params {
		found := false
		v, ok := p.Val.(map[string]interface{})
		if !ok {
			t.Errorf("Expected map[string]interface{}, got %T", p.Val)
			continue
		}
		if len(v) != 3 {
			t.Errorf("Expected length 3, got %d", len(v))
			continue
		}
		vs, ok := v["str"]
		if !ok {
			t.Errorf(`Epected field "s", but not found: %#v`, v)
			continue
		}
		vi, ok := v["Int"]
		if !ok {
			t.Errorf(`Epected field "i", but not found: %#v`, v)
			continue
		}
		vb, ok := v["b"]
		if !ok {
			t.Errorf(`Epected field "b", but not found: %#v`, v)
			continue
		}
		name := fmt.Sprintf("str_%s_int_%v_b_%v", vs, vi, vb)
		if name != p.Name {
			t.Errorf("Expected name %s, got %s", name, p.Name)
		}
		for i, e := range expected {
			if e[0] == vs && vi == e[1] && vb == e[2] {
				expected = append(expected[:i], expected[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Unexpected item: %s", v)
		}
	}
	if len(expected) != 0 {
		t.Errorf("Missing %d expected items.", len(expected))
		for i, e := range expected {
			t.Errorf("%d-th missing item: %s", i, e)
		}
	}
}

func TestCombineIgnoreEmpty(t *gotesting.T) {
	params := Combine(NewAxis("a", 1), NewAxis("b"), NewAxis("c", 2))
	if len(params) != 1 {
		t.Errorf("Expected 1 item, got %d items", len(params))
	}
}

func TestCombineNumericNames(t *gotesting.T) {
	type s struct {
		i int
	}
	params := Combine(NewAxis("a", 1, 2), NewAxis("b", &s{i: 42}, &s{i: 43}))
	if len(params) != 4 {
		t.Errorf("Expected 4 items, got %d items", len(params))
	}
	for i, param := range params {
		if param.Name != fmt.Sprintf("%d", i) {
			t.Errorf("Expected %d, got %s", i, param.Name)
		}
	}
}

type customBool bool

func (c customBool) String() string {
	if c {
		return "yes"
	}
	return "no"
}

type testingEnum int

const (
	testingFoo testingEnum = iota
	testingBar
)

func (te testingEnum) String() string {
	switch te {
	case testingFoo:
		return "foo"
	case testingBar:
		return "bar"
	}
	return "(invalid)"
}

func TestCombineCustomName(t *gotesting.T) {
	params := Combine(NewAxis("x", customBool(true), customBool(false)), NewAxis("y", testingFoo, testingBar))
	expected := make(map[string]bool, 4)
	for _, name := range []string{
		"x_yes_y_foo",
		"x_yes_y_bar",
		"x_no_y_foo",
		"x_no_y_bar",
	} {
		expected[name] = true
	}
	for _, p := range params {
		if _, ok := expected[p.Name]; !ok {
			t.Errorf("Unexpected name: %s", p.Name)
		}
		delete(expected, p.Name)
	}
	if len(expected) > 0 {
		unseenNames := make([]string, 0, len(expected))
		for name := range expected {
			unseenNames = append(unseenNames, name)
		}
		t.Errorf("Expected to have %#v, but not seen", unseenNames)
	}
}
