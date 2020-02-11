// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"chromiumos/tast/errors"
)

const (
	testDataSubdir = "data" // subdir relative to test package containing data files

	testNameAttrPrefix   = "name:"   // prefix for auto-added attribute containing test name
	testBundleAttrPrefix = "bundle:" // prefix for auto-added attribute containing bundle name
	testDepAttrPrefix    = "dep:"    // prefix for auto-added attribute containing software dependency

	exitTimeout     = 3 * time.Second  // extra time granted to test-related funcs to exit
	preTestTimeout  = 15 * time.Second // timeout for TestConfig.PreTestFunc
	postTestTimeout = 15 * time.Second // timeout for TestConfig.PostTestFunc
)

// TestInstance represents a test instance registered to the framework.
//
// A test instance is the unit of "tests" exposed to outside of the framework.
// For example, in the command line of the "tast" command, users specify
// which tests to run by names of test instances. Single testing.AddTest call
// may register multiple test instances at once if testing.Test passed to the
// function has non-empty Params field.
//
// While this struct can be marshaled to a JSON object, unmarshaling that object
// will not yield a runnable TestInstance struct; Func will not be present.
// TODO(crbug.com/984387): Split JSON part into another struct.
type TestInstance struct {
	// Name specifies the test's name as "category.TestName".
	// The name is derived from Func's package and function name.
	// The category is the final component of the package.
	Name string `json:"name"`

	// Pkg contains the Go package in which Func is located.
	Pkg string `json:"pkg"`

	// ExitTimeout contains the maximum duration to wait for Func to exit after a timeout.
	// The context passed to Func has a deadline based on Timeout, but Tast waits for an additional ExitTimeout to elapse
	// before reporting that the test has timed out; this gives the test function time to return after it
	// sees that its context has expired before an additional error is added about the timeout.
	// This is exposed for unit tests and should almost always be omitted when defining tests;
	// a reasonable default will be used.
	ExitTimeout time.Duration `json:"-"`

	// AdditionalTime contains an upper bound of additional time allocated to the test.
	AdditionalTime time.Duration `json:"additionalTime,omitEmpty"`

	// Val contains the value inherited from the expanded Param struct for a parameterized test case.
	// This can be retrieved from testing.State.Param().
	Val interface{} `json:"-"`

	// PreCtx is a context that lives as long as the precondition.
	PreCtx context.Context `json:"-"`
	// PreCtxCancel cancels PreCtx.
	PreCtxCancel func() `json:"-"`

	// Following fields are copied from testing.Test struct.
	// See the documents of the struct.

	Func         TestFunc      `json:"-"`
	Desc         string        `json:"desc"`
	Contacts     []string      `json:"contacts"`
	Attr         []string      `json:"attr"`
	Data         []string      `json:"data"`
	Vars         []string      `json:"vars,omitempty"`
	SoftwareDeps []string      `json:"softwareDeps,omitempty"`
	ServiceDeps  []string      `json:"serviceDeps,omitempty"`
	Pre          Precondition  `json:"-"`
	Timeout      time.Duration `json:"timeout"`
}

// newTestInstance creates a TestInstance instance from the given Test info.
// t must be validated one.
// For a parameterized test case, p is specified. p must be contained in t.Params.
func newTestInstance(t *Test, p *Param) (*TestInstance, error) {
	info, err := getTestFuncInfo(t.Func)
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%s.%s", info.category, info.name)

	attrs := append([]string(nil), t.Attr...)
	data := append([]string(nil), t.Data...)
	swDeps := append([]string(nil), t.SoftwareDeps...)
	pre := t.Pre
	timeout := t.Timeout
	var val interface{}
	if p != nil {
		if p.Name != "" {
			name = fmt.Sprintf("%s.%s", name, p.Name)
		}
		attrs = append(attrs, p.ExtraAttr...)
		data = append(data, p.ExtraData...)
		swDeps = append(swDeps, p.ExtraSoftwareDeps...)
		val = p.Val

		// Only one precondition can be defined.
		if t.Pre != nil && p.Pre != nil {
			return nil, errors.New("Param has Pre specified and its enclosing Test also has Pre specified," +
				"but only one can be specified")
		}
		if p.Pre != nil {
			pre = p.Pre
		}

		// Only one timeout can be set.
		if t.Timeout != 0 && p.Timeout != 0 {
			return nil, errors.New("Param has Timeout specified and its enclosing Test also has Timeout specified, but only one can be specified")
		}
		if p.Timeout != 0 {
			timeout = p.Timeout
		}
	}

	aattrs, err := autoAttrs(name, info.pkg, swDeps)
	if err != nil {
		return nil, err
	}

	return &TestInstance{
		Name:           name,
		Pkg:            info.pkg,
		AdditionalTime: additionalTime(pre),
		Val:            val,
		Func:           t.Func,
		Desc:           t.Desc,
		Contacts:       append([]string(nil), t.Contacts...),
		Attr:           append(aattrs, attrs...),
		Data:           data,
		Vars:           append([]string(nil), t.Vars...),
		SoftwareDeps:   swDeps,
		ServiceDeps:    append([]string(nil), t.ServiceDeps...),
		Pre:            pre,
		Timeout:        timeout,
	}, nil
}

// autoAttrs adds automatically-generated attributes to Attr.
func autoAttrs(name, pkg string, softwareDeps []string) ([]string, error) {
	if name == "" {
		return nil, errors.New("test name is empty")
	}
	if pkg == "" {
		return nil, errors.New("test package is empty")
	}

	result := []string{testNameAttrPrefix + name}
	if comps := strings.Split(pkg, "/"); len(comps) >= 2 {
		result = append(result, testBundleAttrPrefix+comps[len(comps)-2])
	}
	for _, dep := range softwareDeps {
		result = append(result, testDepAttrPrefix+dep)
	}
	return result, nil
}

// additionalTime returns AdditionalTime to include time needed for Precondition and pre-test or post-test functions.
func additionalTime(pre Precondition) time.Duration {
	// We don't know whether a pre-test or post-test func will be specified until the test is run,
	// so err on the side of including the time that would be allocated.
	result := preTestTimeout + postTestTimeout

	// The precondition's timeout applies both when preparing the precondition and when closing it
	// (which we'll need to do if this is the final test using the precondition).
	if pre != nil {
		result += 2 * pre.Timeout()
	}

	return result
}

func (t *TestInstance) clone() *TestInstance {
	ret := &TestInstance{}
	*ret = *t
	ret.Contacts = append([]string(nil), ret.Contacts...)
	ret.Attr = append([]string(nil), ret.Attr...)
	ret.Data = append([]string(nil), ret.Data...)
	ret.Vars = append([]string(nil), ret.Vars...)
	ret.SoftwareDeps = append([]string(nil), ret.SoftwareDeps...)
	ret.ServiceDeps = append([]string(nil), ret.ServiceDeps...)
	return ret
}

// DataDir returns the path to the directory in which files listed in Data will be located,
// relative to the top-level directory containing data files.
func (t *TestInstance) DataDir() string {
	return filepath.Join(t.Pkg, testDataSubdir)
}

// Run runs the test per cfg and blocks until the test has either finished or its deadline is reached,
// whichever comes first.
//
// The time allotted to the test is generally the sum of t.Timeout and t.ExitTimeout, but
// additional time may be allotted for t.Pre.Prepare, t.Pre.Close, cfg.PreTestFunc, and cfg.PostTestFunc.
//
// The test function executes in a goroutine and may still be running if it ignores its deadline;
// the returned value indicates whether the test completed within the allotted time or not.
// ch is only closed after the test function completes, so if false is returned,
// the caller is responsible for reporting that the test timed out.
//
// Stages are executed in the following order:
//	- cfg.PreTestFunc (if non-nil)
//	- t.Pre.Prepare (if t.Pre is non-nil and no errors yet)
//	- t.Func (if no errors yet)
//	- t.Pre.Close (if t.Pre is non-nil and cfg.NextTest.Pre is different)
//	- cfg.PostTestFunc (if non-nil)
func (t *TestInstance) Run(ctx context.Context, ch chan<- Output, cfg *TestConfig) bool {
	// Attach the state to a context so support packages can log to it.
	s := newState(t, ch, cfg)
	ctx = WithTestContext(ctx, s.testContext())

	var stages []stage
	addStage := func(f stageFunc, ctxTimeout, runTimeout time.Duration) {
		stages = append(stages, stage{f, ctxTimeout, runTimeout})
	}

	var postTestHook func(ctx context.Context, s *State)

	// First, perform setup and run the pre-test function.
	addStage(func(ctx context.Context, s *State) {
		// The test bundle is responsible for ensuring t.Timeout is nonzero before calling Run,
		// but we call s.Fatal instead of panicking since it's arguably nicer to report individual
		// test failures instead of aborting the entire run.
		if t.Timeout <= 0 {
			s.Fatal("Invalid timeout ", t.Timeout)
		}

		if cfg.OutDir != "" { // often left blank for unit tests
			if err := os.MkdirAll(cfg.OutDir, 0755); err != nil {
				s.Fatal("Failed to create output dir: ", err)
			}
			// Make the directory world-writable so that tests can create files as other users,
			// and set the sticky bit to prevent users from deleting other users' files.
			// (The mode passed to os.MkdirAll is modified by umask, so we need an explicit chmod.)
			if err := os.Chmod(cfg.OutDir, 0777|os.ModeSticky); err != nil {
				s.Fatal("Failed to set permissions on output dir: ", err)
			}
		}

		// Make sure all required data files exist.
		for _, fn := range t.Data {
			fp := s.DataPath(fn)
			if _, err := os.Stat(fp); err == nil {
				continue
			}
			ep := fp + ExternalErrorSuffix
			if data, err := ioutil.ReadFile(ep); err == nil {
				s.Errorf("Required data file %s missing: %s", fn, string(data))
			} else {
				s.Errorf("Required data file %s missing", fn)
			}
		}
		if s.HasError() {
			return
		}

		// In remote tests, reconnect to the DUT if needed.
		if s.root.cfg.RemoteData != nil {
			dt := s.DUT()
			if !dt.Connected(ctx) {
				s.Log("Reconnecting to DUT")
				if err := dt.Connect(ctx); err != nil {
					s.Fatal("Failed to reconnect to DUT: ", err)
				}
			}
		}

		if cfg.PreTestFunc != nil {
			postTestHook = cfg.PreTestFunc(ctx, s)
		}
	}, preTestTimeout, preTestTimeout+exitTimeout)

	// Prepare the test's precondition (if any) if setup was successful.
	if t.Pre != nil {
		addStage(func(ctx context.Context, s *State) {
			if s.HasError() {
				return
			}
			s.Logf("Preparing precondition %q", t.Pre)

			if t.PreCtx == nil {
				tc := &TestContext{
					Logger: func(msg string) { s.Log(msg) },
				}

				t.PreCtx, t.PreCtxCancel = context.WithCancel(WithTestContext(context.Background(), tc))
			}

			if cfg.NextTest != nil && cfg.NextTest.Pre == t.Pre {
				cfg.NextTest.PreCtx = t.PreCtx
				cfg.NextTest.PreCtxCancel = t.PreCtxCancel
			}

			s.inPre = true
			defer func() { s.inPre = false }()
			s.root.preCtx = t.PreCtx
			s.root.preValue = t.Pre.(preconditionImpl).Prepare(ctx, s)
		}, t.Pre.Timeout(), t.Pre.Timeout()+exitTimeout)
	}

	// Next, run the test function itself if no errors have been reported so far.
	addStage(func(ctx context.Context, s *State) {
		if !s.HasError() {
			t.Func(ctx, s)
		}
	}, t.Timeout, t.Timeout+timeoutOrDefault(t.ExitTimeout, exitTimeout))

	// If this is the final test using this precondition, close it
	// (even if setup, t.Pre.Prepare, or t.Func failed).
	if t.Pre != nil && (cfg.NextTest == nil || cfg.NextTest.Pre != t.Pre) {
		addStage(func(ctx context.Context, s *State) {
			s.Logf("Closing precondition %q", t.Pre.String())
			s.inPre = true
			defer func() { s.inPre = false }()
			t.Pre.(preconditionImpl).Close(ctx, s)
			if t.PreCtxCancel != nil {
				t.PreCtxCancel()
			}
		}, t.Pre.Timeout(), t.Pre.Timeout()+exitTimeout)
	}

	// Finally, run the post-test functions unconditionally.
	addStage(func(ctx context.Context, s *State) {
		if cfg.PostTestFunc != nil {
			cfg.PostTestFunc(ctx, s)
		}

		if postTestHook != nil {
			postTestHook(ctx, s)
		}
	}, postTestTimeout, postTestTimeout+exitTimeout)

	return runStages(ctx, s, stages)
}

// timeoutOrDefault returns timeout if positive or def otherwise.
func timeoutOrDefault(timeout, def time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	return def
}

func (t *TestInstance) String() string {
	return t.Name
}

// MissingSoftwareDeps returns a sorted list of dependencies from SoftwareDeps
// that aren't present on the DUT (per the passed-in features list).
func (t *TestInstance) MissingSoftwareDeps(features []string) []string {
	var missing []string
DepLoop:
	for _, d := range t.SoftwareDeps {
		for _, f := range features {
			if d == f {
				continue DepLoop
			}
		}
		missing = append(missing, d)
	}
	sort.Strings(missing)
	return missing
}

// SortTests sorts tests, primarily by ascending precondition name
// (with tests with no preconditions coming first) and secondarily by ascending test name.
func SortTests(tests []*TestInstance) {
	sort.Slice(tests, func(i, j int) bool {
		ti := tests[i]
		tj := tests[j]

		var pi, pj string
		if ti.Pre != nil {
			pi = ti.Pre.String()
		}
		if tj.Pre != nil {
			pj = tj.Pre.String()
		}

		if pi != pj {
			return pi < pj
		}
		return ti.Name < tj.Name
	})
}

// WriteTestsAsJSON marshals ts to JSON and writes the resulting data to w.
func WriteTestsAsJSON(w io.Writer, ts []*TestInstance) error {
	b, err := json.Marshal(ts)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}
