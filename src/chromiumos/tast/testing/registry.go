// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
)

// Registry holds tests.
type Registry struct {
	allTests  []*TestCase
	testNames map[string]struct{} // names of registered tests
}

// NewRegistry returns a new test registry.
func NewRegistry() *Registry {
	return &Registry{
		allTests:  make([]*TestCase, 0),
		testNames: make(map[string]struct{}),
	}
}

// AddTest adds t to the registry.
func (r *Registry) AddTest(t *Test) error {
	if err := validateTest(t); err != nil {
		return err
	}
	if len(t.Params) == 0 {
		tc, err := newTestCase(t, nil)
		if err != nil {
			return err
		}
		return r.AddTestCase(tc)
	}

	for _, p := range t.Params {
		tc, err := newTestCase(t, &p)
		if err != nil {
			return err
		}
		if err := r.AddTestCase(tc); err != nil {
			return err
		}
	}
	return nil
}

// AddTestCase adds t to the registry.
// TODO(crbug.com/985381): Consider to hide the method for better encapsulation.
func (r *Registry) AddTestCase(t *TestCase) error {
	t = t.clone()
	if _, ok := r.testNames[t.Name]; ok {
		return fmt.Errorf("test %q already registered", t.Name)
	}
	r.allTests = append(r.allTests, t)
	r.testNames[t.Name] = struct{}{}
	return nil
}

// AllTests returns copies of all registered tests.
func (r *Registry) AllTests() []*TestCase {
	ts := make([]*TestCase, len(r.allTests))
	for i, t := range r.allTests {
		ts[i] = t.clone()
	}
	return ts
}
