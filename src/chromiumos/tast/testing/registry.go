// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
)

// Registry holds tests and services.
type Registry struct {
	allPres     map[string]PreconditionV2
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

// AddPreconditionV2 adds pre to the registory.
// It only checks name duplication, but doesn't check cyclic deps.
// Checking cyclic deps is the tast binary's responsibility.
func (r *Registry) AddPreconditionV2(pre PreconditionV2) error {
	if _, ok := pre.(preconditionV2Impl); !ok {
		return fmt.Errorf("precondition V2 %s does not implement preconditionV2Impl", pre)
	}
	s := pre.String()
	if s == "" {
		return fmt.Error("empty precondition name is not allowed")
	}
	if _, ok := r.allPres[s]; ok {
		return fmt.Errorf("precondition V2 %s is already registered", pre)
	}
	r.allPres[s] = pre
	return nil
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
	r.allTests = append(r.allTests, t)
	r.testNames[t.Name] = struct{}{}
	return nil
}

// AddService adds s to the registry.
func (r *Registry) AddService(s *Service) error {
	r.allServices = append(r.allServices, s)
	return nil
}

// AllPreconditionV2s returns copies of all registered V2 preconditions.
func (r *Registry) AllPreconditionV2s() map[string]PreconditionV2 {
	ps := make(map[string]PreconditionV2)
	for k, v := range r.allPres {
		ps[k] = v
	}
	return ps
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
