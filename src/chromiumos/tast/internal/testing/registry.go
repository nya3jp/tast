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
	allPres     map[string]Precondition
	allFixtures map[string]*Fixture
}

// NewRegistry returns a new test registry.
func NewRegistry() *Registry {
	return &Registry{
		testNames:   make(map[string]struct{}),
		allPres:     make(map[string]Precondition),
		allFixtures: make(map[string]*Fixture),
	}
}

// AddTest adds t to the registry.
func (r *Registry) AddTest(t *Test) error {
	tis, err := instantiate(t)
	if err != nil {
		return err
	}
	for _, ti := range tis {
		if err := r.AddTestInstance(ti); err != nil {
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
	// Ensure equality of preconditions with the same name.
	if t.Pre != nil {
		name := t.Pre.String()
		if pre, ok := r.allPres[name]; !ok {
			r.allPres[name] = t.Pre
		} else if pre != t.Pre {
			return fmt.Errorf("precondition %s has multiple instances", name)
		}
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

// AddFixture adds f to the registry.
func (r *Registry) AddFixture(f *Fixture) error {
	if _, ok := r.allFixtures[f.Name]; ok {
		return fmt.Errorf("fixture %q already registered", f.Name)
	}
	r.allFixtures[f.Name] = f
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

// AllFixtures returns copies of all registered fixtures.
func (r *Registry) AllFixtures() map[string]*Fixture {
	fs := make(map[string]*Fixture)
	for name, f := range r.allFixtures {
		fs[name] = f
	}
	return fs
}
