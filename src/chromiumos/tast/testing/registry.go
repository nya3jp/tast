// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
)

// Registry holds tests and services.
type Registry struct {
	allTests    []*TestInstance
	testNames   map[string]struct{} // names of registered tests
	allServices []*Service
}

// NewRegistry returns a new test registry.
func NewRegistry() *Registry {
	return &Registry{
		testNames: make(map[string]struct{}),
	}
}

// AddTest adds t to the registry.
func (r *Registry) AddTest(t *Test) error {
	if err := validateTest(t); err != nil {
		return err
	}
	if len(t.Params) == 0 {
		tc, err := newTestInstance(t, nil)
		if err != nil {
			return err
		}
		return r.AddTestInstance(tc)
	}

	for _, p := range t.Params {
		tc, err := newTestInstance(t, &p)
		if err != nil {
			return err
		}
		if err := r.AddTestInstance(tc); err != nil {
			return err
		}
	}
	return nil
}

// AddTestInstance adds t to the registry.
// TODO(crbug.com/985381): Consider to hide the method for better encapsulation.
func (r *Registry) AddTestInstance(t *TestInstance) error {
	t = t.clone()
	if _, ok := r.testNames[t.Name]; ok {
		return fmt.Errorf("test %q already registered", t.Name)
	}
	r.allTests = append(r.allTests, t)
	r.testNames[t.Name] = struct{}{}
	return nil
}

// AddService adds s to the registry.
func (r *Registry) AddService(s *Service) error {
	r.allServices = append(r.allServices, s)
	return nil
}

// AllTests returns copies of all registered tests.
func (r *Registry) AllTests() []*TestInstance {
	ts := make([]*TestInstance, len(r.allTests))
	for i, t := range r.allTests {
		ts[i] = t.clone()
	}
	return ts
}

// AllServices returns copies of all registered services.
func (r *Registry) AllServices() []*Service {
	return append(([]*Service)(nil), r.allServices...)
}
