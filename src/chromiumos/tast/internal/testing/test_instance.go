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
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	testpb "go.chromium.org/chromiumos/config/go/api/test/metadata/v1"
	"go.chromium.org/chromiumos/infra/proto/go/device"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/testing/hwdep"
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

	// Val contains the value inherited from the expanded Param struct for a parameterized test case.
	// This can be retrieved from testing.State.Param().
	Val interface{} `json:"-"`

	// PreCtx is a context that lives as long as the precondition.
	PreCtx context.Context `json:"-"`
	// PreCtxCancel cancels PreCtx.
	PreCtxCancel func() `json:"-"`

	// Following fields are copied from testing.Test struct.
	// See the documents of the struct.

	Func         TestFunc `json:"-"`
	Desc         string   `json:"desc"`
	Contacts     []string `json:"contacts"`
	Attr         []string `json:"attr"`
	Data         []string `json:"data"`
	Vars         []string `json:"vars,omitempty"`
	SoftwareDeps []string `json:"softwareDeps,omitempty"`
	// HardwareDeps field is not in the protocol yet. When the scheduler in infra is
	// implemented, it is needed.
	HardwareDeps hwdep.Deps    `json:"-"`
	ServiceDeps  []string      `json:"serviceDeps,omitempty"`
	Pre          Precondition  `json:"-"`
	Timeout      time.Duration `json:"timeout"`
}

// instantiate creates one or more TestInstance from t.
func instantiate(t *Test) ([]*TestInstance, error) {
	if err := t.validate(); err != nil {
		return nil, err
	}

	// Empty Params is equivalent to one Param with all default values.
	ps := t.Params
	if len(ps) == 0 {
		ps = []Param{{}}
	}

	tis := make([]*TestInstance, 0, len(ps))
	for _, p := range ps {
		ti, err := newTestInstance(t, &p)
		if err != nil {
			return nil, err
		}
		tis = append(tis, ti)
	}

	return tis, nil
}

func newTestInstance(t *Test, p *Param) (*TestInstance, error) {
	info, err := getTestFuncInfo(t.Func)
	if err != nil {
		return nil, err
	}

	if err := validateFileName(info.name, filepath.Base(info.file)); err != nil {
		return nil, err
	}

	name := fmt.Sprintf("%s.%s", info.category, info.name)
	if p.Name != "" {
		name += "." + p.Name
	}
	if err := validateName(name); err != nil {
		return nil, err
	}

	manualAttrs := append(append([]string(nil), t.Attr...), p.ExtraAttr...)
	if err := validateManualAttr(manualAttrs); err != nil {
		return nil, err
	}

	data := append(append([]string(nil), t.Data...), p.ExtraData...)
	if err := validateData(t.Data); err != nil {
		return nil, err
	}

	if err := validateVars(info.category, info.name, t.Vars); err != nil {
		return nil, err
	}

	swDeps := append(append([]string(nil), t.SoftwareDeps...), p.ExtraSoftwareDeps...)
	hwDeps := hwdep.Merge(t.HardwareDeps, p.ExtraHardwareDeps)
	if err := hwDeps.Validate(); err != nil {
		return nil, err
	}

	attrs := append(manualAttrs, autoAttrs(name, info.pkg, swDeps)...)
	attrs = modifyAttrsForCompat(attrs)
	if err := validateAttr(attrs); err != nil {
		return nil, err
	}

	pre := t.Pre
	if p.Pre != nil {
		if t.Pre != nil {
			return nil, errors.New("Param has Pre specified and its enclosing Test also has Pre specified," +
				"but only one can be specified")
		}
		pre = p.Pre
	}
	if pre != nil {
		if _, ok := pre.(PreconditionImpl); !ok {
			return nil, fmt.Errorf("precondition %s does not implement preconditionImpl", pre)
		}
	}

	timeout := t.Timeout
	if p.Timeout != 0 {
		if t.Timeout != 0 {
			return nil, errors.New("Param has Timeout specified and its enclosing Test also has Timeout specified, but only one can be specified")
		}
		timeout = p.Timeout
	}
	if timeout < 0 {
		return nil, fmt.Errorf("timeout is negative (%v)", timeout)
	}

	return &TestInstance{
		Name:         name,
		Pkg:          info.pkg,
		Val:          p.Val,
		Func:         t.Func,
		Desc:         t.Desc,
		Contacts:     append([]string(nil), t.Contacts...),
		Attr:         attrs,
		Data:         data,
		Vars:         append([]string(nil), t.Vars...),
		SoftwareDeps: swDeps,
		HardwareDeps: hwDeps,
		ServiceDeps:  append([]string(nil), t.ServiceDeps...),
		Pre:          pre,
		Timeout:      timeout,
	}, nil
}

// autoAttrs returns automatically-generated attributes.
func autoAttrs(name, pkg string, softwareDeps []string) []string {
	attrs := []string{testNameAttrPrefix + name}
	if comps := strings.Split(pkg, "/"); len(comps) >= 2 {
		attrs = append(attrs, testBundleAttrPrefix+comps[len(comps)-2])
	}
	for _, dep := range softwareDeps {
		attrs = append(attrs, testDepAttrPrefix+dep)
	}
	return attrs
}

// testFuncInfo contains information about a TestFunc.
type testFuncInfo struct {
	pkg      string // package name, e.g. "chromiumos/tast/local/bundles/cros/ui"
	category string // Tast category name, e.g. "ui". The last component of pkg
	name     string // function name, e.g. "ChromeLogin"
	file     string // full source path, e.g. "/home/user/chromeos/src/platform/tast-tests/.../ui/chrome_login.go"
}

// getTestFuncInfo returns info about f.
func getTestFuncInfo(f TestFunc) (*testFuncInfo, error) {
	if f == nil {
		return nil, errors.New("Func is nil")
	}
	pc := reflect.ValueOf(f).Pointer()
	rf := runtime.FuncForPC(pc)
	if rf == nil {
		return nil, errors.New("failed to get function from PC")
	}
	p := strings.SplitN(rf.Name(), ".", 2)
	if len(p) != 2 {
		return nil, fmt.Errorf("didn't find package.function in %q", rf.Name())
	}

	cs := strings.Split(p[0], "/")
	if len(cs) < 2 {
		return nil, fmt.Errorf("failed to split package %q into at least two components", p[0])
	}

	info := &testFuncInfo{
		pkg:      p[0],
		category: cs[len(cs)-1],
		name:     p[1],
	}
	info.file, _ = rf.FileLine(pc)
	return info, nil
}

// testNameRegexp validates test names, which should consist of a package name,
// a period, the name of the exported test function, followed optionally by
// a period and the name of the parameter.
var testNameRegexp = regexp.MustCompile(`^[a-z][a-z0-9]*\.[A-Z][A-Za-z0-9]*(?:\.[a-z0-9_]+)?$`)

func validateName(name string) error {
	if !testNameRegexp.MatchString(name) {
		return fmt.Errorf("invalid test name %q", name)
	}
	return nil
}

// testWordRegexp validates an individual word in a test function name.
// See checkFuncNameAgainstFilename for details.
var testWordRegexp = regexp.MustCompile("^[A-Z0-9]+[a-z0-9]*[A-Z0-9]*$")

func validateFileName(funcName, filename string) error {
	if strings.ToLower(filename) != filename {
		return fmt.Errorf("filename %q isn't lowercase", filename)
	}
	const goExt = ".go"
	if filepath.Ext(filename) != goExt {
		return fmt.Errorf("filename %q doesn't have extension %q", filename, goExt)
	}

	// First, split the name into words based on underscores in the filename.
	funcIdx := 0
	fileWords := strings.Split(filename[:len(filename)-len(goExt)], "_")
	for _, fileWord := range fileWords {
		// Disallow repeated underscores.
		if len(fileWord) == 0 {
			return fmt.Errorf("empty word in filename %q", filename)
		}

		// Extract the characters from the function name corresponding to the word from the filename.
		if funcIdx+len(fileWord) > len(funcName) {
			return fmt.Errorf("name %q doesn't include all of filename %q", funcName, filename)
		}
		funcWord := funcName[funcIdx : funcIdx+len(fileWord)]
		if strings.ToLower(funcWord) != strings.ToLower(fileWord) {
			return fmt.Errorf("word %q at %q[%d] doesn't match %q in filename %q", funcWord, funcName, funcIdx, fileWord, filename)
		}

		// Test names are taken from Go function names, so they should follow Go's naming conventions.
		// Generally speaking, that means camel case with acronyms fully capitalized (although we can't catch
		// miscapitalized acronyms here, as we don't know if a given word is an acronym or not).
		// Every word should begin with either an uppercase letter or a digit.
		// Multiple leading or trailing uppercase letters are allowed to permit filename -> func-name pairings like
		// dbus.go -> "DBus", webrtc.go -> "WebRTC", and crosvm.go -> "CrosVM".
		// Note that this also permits incorrect filenames like loadurl.go for "LoadURL", but that's not something code can prevent.
		if !testWordRegexp.MatchString(funcWord) {
			return fmt.Errorf("word %q at %q[%d] should probably be %q (acronyms also allowed at beginning and end)",
				funcWord, funcName, funcIdx, strings.Title(strings.ToLower(funcWord)))
		}

		funcIdx += len(funcWord)
	}

	if funcIdx < len(funcName) {
		return fmt.Errorf("name %q has extra suffix %q not in filename %q", funcName, funcName[funcIdx:], filename)
	}

	return nil
}

func isAutoAttr(attr string) bool {
	for _, pre := range []string{testNameAttrPrefix, testBundleAttrPrefix, testDepAttrPrefix} {
		if strings.HasPrefix(attr, pre) {
			return true
		}
	}
	return false
}

func validateManualAttr(attr []string) error {
	for _, a := range attr {
		if isAutoAttr(a) {
			return fmt.Errorf("attribute %q has reserved prefix", a)
		}
		if a == "disabled" {
			return errors.New("the disabled attribute is deprecated; remove group:* attributes instead")
		}
	}
	return nil
}

func validateAttr(attr []string) error {
	if err := checkKnownAttrs(attr); err != nil {
		return err
	}
	return nil
}

func validateData(data []string) error {
	for _, p := range data {
		if p != filepath.Clean(p) || strings.HasPrefix(p, ".") || strings.HasPrefix(p, "/") {
			return fmt.Errorf("data path %q is invalid", p)
		}
	}
	return nil
}

var validVarLastPartRE = regexp.MustCompile("[a-zA-Z][0-9A-Za-z_]*")

func validateVars(category, name string, vars []string) error {
	for _, v := range vars {
		parts := strings.Split(v, ".")
		// Allow global variables e.g. "servo".
		if len(parts) == 1 {
			continue
		}
		if len(parts) == 2 && parts[0] == category && validVarLastPartRE.MatchString(parts[1]) {
			continue
		}
		if len(parts) == 3 && parts[0] == category && parts[1] == name && validVarLastPartRE.MatchString(parts[2]) {
			continue
		}
		return fmt.Errorf("valiable name %s violates our naming convention defined in https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Runtime-variables", v)
	}
	return nil
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
				// Associate PreCtx with TestContext for the first test.
				t.PreCtx, t.PreCtxCancel = context.WithCancel(WithTestContext(context.Background(), s.testContext()))
			}

			if cfg.NextTest != nil && cfg.NextTest.Pre == t.Pre {
				cfg.NextTest.PreCtx = t.PreCtx
				cfg.NextTest.PreCtxCancel = t.PreCtxCancel
			}

			s.inPre = true
			defer func() { s.inPre = false }()
			s.root.preCtx = t.PreCtx
			s.root.preValue = t.Pre.(PreconditionImpl).Prepare(ctx, s)
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
			t.Pre.(PreconditionImpl).Close(ctx, s)
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

// SkipReason represents the reasons why the test needs to be skipped.
type SkipReason struct {
	// MissingSoftwareDeps contains a list of features which is required by the test,
	// but not satisfied under the current DUT.
	MissingSoftwareDeps []string

	// HardwareDepsUnsatisfiedReasons contains a list of error messages, why
	// some hardware dependencies were not satisfied.
	HardwareDepsUnsatisfiedReasons []string
}

// ShouldRun returns whether this test should run under the current testing environment.
// In case of not, in addition, the reason why it should be skipped is also returned.
func (t *TestInstance) ShouldRun(features []string, dc *device.Config) (bool, *SkipReason) {
	missing := t.MissingSoftwareDeps(features)
	var hwReasons []string
	if err := t.HardwareDeps.Satisfied(dc); err != nil {
		for _, r := range err.Reasons {
			hwReasons = append(hwReasons, r.Error())
		}
	}
	if len(missing) > 0 || len(hwReasons) > 0 {
		return false, &SkipReason{missing, hwReasons}
	}
	return true, nil
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

// Proto converts test metadata of TestInstance into a protobuf message.
func (t *TestInstance) Proto() *testpb.Test {
	r := testpb.Test{
		Name:          t.Name,
		Informational: &testpb.Informational{},
	}
	for _, a := range t.Attr {
		r.Attributes = append(r.Attributes, &testpb.Attribute{Name: a})
	}
	// TODO(crbug.com/1047561): Fill r.DUTCondition.
	for _, email := range t.Contacts {
		c := testpb.Contact{Type: &testpb.Contact_Email{Email: email}}
		r.Informational.Authors = append(r.Informational.Authors, &c)
	}
	return &r
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

// WriteTestsAsProto exports test metadata in the protobuf format defined by infra.
func WriteTestsAsProto(w io.Writer, ts []*TestInstance) error {
	var result testpb.RemoteTestDriver
	result.Name = "remoteTestDrivers/tast"
	for _, src := range ts {
		result.Tests = append(result.Tests, src.Proto())
	}
	d, err := proto.Marshal(&result)
	if err != nil {
		return errors.Wrap(err, "Failed to marshalize the proto")
	}
	_, err = w.Write(d)
	return err
}
