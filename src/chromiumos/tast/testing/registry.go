// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
)

// Registry holds tests and services.
type Registry struct {
	allTests    []*RunnableTest
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
	sts, err := t.settle()
	if err != nil {
		return err
	}

	for _, st := range sts {
		tc, err := newRunnableTest(st)
		if err != nil {
			return err
		}
		if err := r.AddRunnableTest(tc); err != nil {
			return err
		}
	}
	return nil
}

// AddRunnableTest adds t to the registry.
// TODO(crbug.com/985381): Consider to hide the method for better encapsulation.
func (r *Registry) AddRunnableTest(t *RunnableTest) error {
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
func (r *Registry) AllTests() []*RunnableTest {
	ts := make([]*RunnableTest, len(r.allTests))
	for i, t := range r.allTests {
		ts[i] = t.clone()
	}
	return ts
}

// AllServices returns copies of all registered services.
func (r *Registry) AllServices() []*Service {
	return append(([]*Service)(nil), r.allServices...)
}
