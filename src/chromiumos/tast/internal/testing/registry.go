// Copyright 2017 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/packages"
)

// Registry holds tests and services.
type Registry struct {
	name         string
	errors       []error
	allTests     []*TestInstance
	testNames    map[string]struct{} // names of registered tests
	allServices  []*Service
	allPres      map[string]Precondition
	allFixtures  map[string]*FixtureInstance
	allVars      map[string]Var    // all registered global runtime variables
	varRawValues map[string]string // raw values of global runtime variables
}

// NewRegistry returns a new test registry.
func NewRegistry(name string) *Registry {
	return &Registry{
		name:        name,
		testNames:   make(map[string]struct{}),
		allPres:     make(map[string]Precondition),
		allFixtures: make(map[string]*FixtureInstance),
		allVars:     make(map[string]Var),
	}
}

// Name returns the name of the registry.
func (r *Registry) Name() string {
	return r.name
}

// Errors returns errors generated by registration method calls.
func (r *Registry) Errors() []error {
	return append([]error(nil), r.errors...)
}

// RecordError records err as a registration error if it is not nil.
func (r *Registry) RecordError(err error) {
	if err == nil {
		return
	}
	file, line := userCaller()
	r.errors = append(r.errors, errors.Wrapf(err, "%s:%d", file, line))
}

// AddTest adds t to the registry.
func (r *Registry) AddTest(t *Test) {
	r.RecordError(func() error {
		tis, err := instantiate(t)
		if err != nil {
			return err
		}
		for _, ti := range tis {
			r.AddTestInstance(ti)
		}
		return nil
	}())
}

// AddTestInstance adds t to the registry.
// In contrast to AddTest, AddTestInstance registers an instantiated test almost
// as-is. It just associates the test with this registry so that the test cannot
// be registered to other registries.
// AddTestInstance should be called only by unit tests. It must not be
// accessible by user code.
func (r *Registry) AddTestInstance(t *TestInstance) {
	r.RecordError(func() error {
		if _, ok := r.testNames[t.Name]; ok {
			return fmt.Errorf("test %q already registered", t.Name)
		}
		if t.Bundle != "" {
			return fmt.Errorf("TestInstance %q cannot be added to bundle %q since it is already added to bundle %q", t.Name, r.name, t.Bundle)
		}
		t.Bundle = r.name
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
	}())
}

// AddService adds s to the registry.
func (r *Registry) AddService(s *Service) {
	r.allServices = append(r.allServices, s)
}

// AddFixture adds f to the registry.
func (r *Registry) AddFixture(f *Fixture, pkg string) {
	r.RecordError(func() error {
		fi, err := f.instantiate(pkg)
		if err != nil {
			return err
		}
		r.AddFixtureInstance(fi)
		return nil
	}())
}

// AddFixtureInstance adds f to the registry.
// In contrast to AddFixture, AddFixtureInstance registers an instantiated
// fixture almost as-is. It just associates the fixture with this registry so
// that the fixture cannot be registered to other registries.
// AddFixtureInstance should be called only by unit tests. It must not be
// accessible by user code.
func (r *Registry) AddFixtureInstance(f *FixtureInstance) {
	r.RecordError(func() error {
		if _, ok := r.allFixtures[f.Name]; ok {
			return fmt.Errorf("fixture %q already registered", f.Name)
		}
		if f.Bundle != "" {
			return fmt.Errorf("FixtureInstance %q cannot be added to bundle %q since it is already added to bundle %q", f.Name, r.name, f.Bundle)
		}
		f.Bundle = r.name
		r.allFixtures[f.Name] = f
		return nil
	}())
}

// AddVar adds global variables to the registry.
func (r *Registry) AddVar(v Var) {
	r.RecordError(func() error {
		name := v.Name()
		if _, ok := r.allVars[name]; ok {
			return fmt.Errorf("global runtime variable %q has already been registered", v.Name())
		}
		r.allVars[name] = v
		return nil
	}())
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
func (r *Registry) AllFixtures() map[string]*FixtureInstance {
	fs := make(map[string]*FixtureInstance)
	for name, f := range r.allFixtures {
		fs[name] = f
	}
	return fs
}

// AllVars returns copies of all registered all runtime variables.
func (r *Registry) AllVars() []Var {
	var vars []Var
	for _, f := range r.allVars {
		vars = append(vars, f)
	}
	return vars
}

// userCaller finds the caller of a registration function. It ignores framework
// packages.
func userCaller() (file string, line int) {
	for skip := 1; ; skip++ {
		pc, file, line, _ := runtime.Caller(skip)
		f := runtime.FuncForPC(pc)
		name := packages.Normalize(f.Name())
		if strings.HasPrefix(name, packages.FrameworkPrefix+"internal/") ||
			strings.HasPrefix(name, packages.FrameworkPrefix+"testing.") {
			continue
		}
		return file, line
	}
}

// InitializeVars initializes all registered global variables.
func (r *Registry) InitializeVars(values map[string]string) error {
	if r.varRawValues != nil {
		if !cmp.Equal(values, r.varRawValues, cmpopts.EquateEmpty()) {
			return errors.New("global runtime variables can only be initialized once")
		}
		return nil
	}
	// Save raw values for future comparison.
	r.varRawValues = make(map[string]string)
	for k, v := range values {
		r.varRawValues[k] = v
	}
	// Set value for each variable.
	for _, v := range r.allVars {
		if stringValue, ok := values[v.Name()]; ok {
			if err := v.Unmarshal(stringValue); err != nil {
				return err
			}
		}
	}
	return nil
}
