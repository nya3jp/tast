// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.chromium.org/chromiumos/config/go/test/api"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/dep"
	"chromiumos/tast/internal/packages"
	"chromiumos/tast/internal/protocol"
)

const (
	testDataSubdir = "data" // subdir relative to test package containing data files

	testNameAttrPrefix   = "name:"   // prefix for auto-added attribute containing test name
	testBundleAttrPrefix = "bundle:" // prefix for auto-added attribute containing bundle name
	testDepAttrPrefix    = "dep:"    // prefix for auto-added attribute containing software dependency

	testHarnessPrefix = "tast" // prefix for test id of test metadata
)

// TestInstance represents a test instance registered to the framework.
//
// A test instance is the unit of "tests" exposed to outside of the framework.
// For example, in the command line of the "tast" command, users specify
// which tests to run by names of test instances. Single testing.AddTest call
// may register multiple test instances at once if testing.Test passed to the
// function has non-empty Params field.
type TestInstance struct {
	// Name specifies the test's name as "category.TestName".
	// The name is derived from Func's package and function name.
	// The category is the final component of the package.
	Name string

	// Pkg contains the Go package in which Func is located.
	Pkg string

	// ExitTimeout contains the maximum duration to wait for Func to exit after a timeout.
	// The context passed to Func has a deadline based on Timeout, but Tast waits for an additional ExitTimeout to elapse
	// before reporting that the test has timed out; this gives the test function time to return after it
	// sees that its context has expired before an additional error is added about the timeout.
	// This is exposed for unit tests and should almost always be omitted when defining tests;
	// a reasonable default will be used.
	// TODO(oka): Remove ExitTimeout using CustomGracePeriod in planner.Config .
	ExitTimeout time.Duration

	// Val contains the value inherited from the expanded Param struct for a parameterized test case.
	// This can be retrieved from testing.State.Param().
	Val interface{}

	// Following fields are copied from testing.Test struct.
	// See the documents of the struct.

	Func         TestFunc
	Desc         string
	Contacts     []string
	Attr         []string
	Labels       []string
	SearchFlags  []*protocol.StringPair
	Data         []string
	Vars         []string
	VarDeps      []string
	SoftwareDeps dep.SoftwareDeps
	// HardwareDeps field is not in the protocol yet. When the scheduler in infra is
	// implemented, it is needed.
	HardwareDeps dep.HardwareDeps
	ServiceDeps  []string
	Pre          Precondition
	Fixture      string
	Timeout      time.Duration

	// Bundle is the name of the test bundle this test belongs to.
	// This field is empty initially, and later set when the test is added
	// to testing.Registry.
	Bundle string
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

	if err := validateVars(info.category, info.name, append(append([]string(nil), t.Vars...), t.VarDeps...)); err != nil {
		return nil, err
	}

	swDeps := append(append([]string(nil), t.SoftwareDeps...), p.ExtraSoftwareDeps...)
	hwDeps := dep.MergeHardwareDeps(t.HardwareDeps, p.ExtraHardwareDeps)
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
			return nil, errors.New("Param has Pre specified and its enclosing Test also has Pre specified, but only one can be specified")
		}
		pre = p.Pre
	}
	if err := validatePre(pre); err != nil {
		return nil, err
	}

	fixt := t.Fixture
	if p.Fixture != "" {
		if t.Fixture != "" {
			return nil, errors.New("Param has Fixture specified and its enclosing Test also has Fixture specified, but only one can be specified")
		}
		fixt = p.Fixture
	}

	if pre != nil && fixt != "" {
		return nil, errors.New("Fixture and Pre cannot be set simultaneously")
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

	labels := append(append([]string(nil), t.Labels...), p.ExtraLabels...)
	searchFlags := append(append([]*protocol.StringPair(nil), t.SearchFlags...), p.ExtraSearchFlags...)

	return &TestInstance{
		Name:         name,
		Pkg:          info.pkg,
		Val:          p.Val,
		Func:         t.Func,
		Desc:         t.Desc,
		Contacts:     append([]string(nil), t.Contacts...),
		Attr:         attrs,
		Labels:       labels,
		SearchFlags:  searchFlags,
		Data:         data,
		Vars:         append([]string(nil), t.Vars...),
		VarDeps:      append([]string(nil), t.VarDeps...),
		SoftwareDeps: swDeps,
		HardwareDeps: hwDeps,
		ServiceDeps:  append([]string(nil), t.ServiceDeps...),
		Pre:          pre,
		Fixture:      fixt,
		Timeout:      timeout,
	}, nil
}

// autoAttrs returns automatically-generated attributes.
func autoAttrs(name, pkg string, deps dep.SoftwareDeps) []string {
	attrs := []string{testNameAttrPrefix + name}
	if comps := strings.Split(pkg, "/"); len(comps) >= 2 {
		attrs = append(attrs, testBundleAttrPrefix+comps[len(comps)-2])
	}
	for _, dep := range deps {
		attrs = append(attrs, testDepAttrPrefix+dep)
	}
	return attrs
}

// testFuncInfo contains information about a TestFunc.
type testFuncInfo struct {
	pkg      string // package name, e.g. "chromiumos/tast/local/bundles/cros/login"
	category string // Tast category name, e.g. "login". The last component of pkg
	name     string // function name, e.g. "Chrome"
	file     string // full source path, e.g. "/home/user/chromeos/src/platform/tast-tests/.../login/chrome.go"
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
	p, name := packages.SplitFuncName(rf.Name())

	cs := strings.Split(p, "/")
	if len(cs) < 2 {
		return nil, fmt.Errorf("failed to split package %q into at least two components", p)
	}

	info := &testFuncInfo{
		pkg:      p,
		category: cs[len(cs)-1],
		name:     name,
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

func validatePre(pre Precondition) error {
	// Precondition must be a pointer type so that it is comparable.
	// https://golang.org/ref/spec#Comparison_operators
	if pre == nil {
		return nil
	}
	v := reflect.ValueOf(pre)
	if v.Kind() != reflect.Ptr {
		return errors.New("precondition must be implemented by a pointer type")
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
		if len(parts) == 2 && validVarLastPartRE.MatchString(parts[1]) {
			continue
		}
		if len(parts) == 3 && parts[0] == category && parts[1] == name && validVarLastPartRE.MatchString(parts[2]) {
			continue
		}
		return fmt.Errorf("variable name %s violates our naming convention defined in https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Runtime-variables", v)
	}

	seen := make(map[string]struct{})
	for _, v := range vars {
		if _, ok := seen[v]; ok {
			return fmt.Errorf("Vars and VarDeps should not contain the same variable %q twice", v)
		}
		seen[v] = struct{}{}
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
	ret.VarDeps = append([]string(nil), ret.VarDeps...)
	ret.SoftwareDeps = append([]string(nil), ret.SoftwareDeps...)
	ret.ServiceDeps = append([]string(nil), ret.ServiceDeps...)
	return ret
}

func (t *TestInstance) String() string {
	return t.Name
}

// Deps dependencies of this test.
func (t *TestInstance) Deps() *dep.Deps {
	return &dep.Deps{
		Var:      t.VarDeps,
		Software: append([]string(nil), t.SoftwareDeps...),
		Hardware: t.HardwareDeps,
	}
}

// Proto converts test metadata of TestInstance into a protobuf message.
func (t *TestInstance) Proto() *api.TestCaseMetadata {
	var tags []*api.TestCase_Tag
	for _, a := range t.Attr {
		tags = append(tags, &api.TestCase_Tag{Value: a})
	}
	var owners []*api.Contact
	for _, email := range t.Contacts {
		owners = append(owners, &api.Contact{Email: email})
	}
	r := api.TestCaseMetadata{
		TestCase: &api.TestCase{
			Id: &api.TestCase_Id{
				Value: testHarnessPrefix + "." + t.Name,
			},
			Name: t.Name,
			Tags: tags,
		},
		TestCaseExec: &api.TestCaseExec{
			TestHarness: &api.TestHarness{
				TestHarnessType: &api.TestHarness_Tast_{
					Tast: &api.TestHarness_Tast{},
				},
			},
		},
		TestCaseInfo: &api.TestCaseInfo{
			Owners: owners,
		},
	}
	return &r
}

// Constraints returns EntityConstraints for this test.
func (t *TestInstance) Constraints() *EntityConstraints {
	return &EntityConstraints{
		allVars: append(append([]string(nil), t.Vars...), t.VarDeps...),
		allData: append([]string(nil), t.Data...),
	}
}

// EntityProto a protocol buffer message representation of TestInstance.
func (t *TestInstance) EntityProto() *protocol.Entity {
	return &protocol.Entity{
		Type:        protocol.EntityType_TEST,
		Name:        t.Name,
		Package:     t.Pkg,
		Attributes:  append([]string(nil), t.Attr...),
		SearchFlags: append([]*protocol.StringPair(nil), t.SearchFlags...),
		Description: t.Desc,
		Fixture:     t.Fixture,
		Dependencies: &protocol.EntityDependencies{
			DataFiles: append([]string(nil), t.Data...),
			Services:  append([]string(nil), t.ServiceDeps...),
		},
		Contacts: &protocol.EntityContacts{
			Emails: append([]string(nil), t.Contacts...),
		},
		LegacyData: &protocol.EntityLegacyData{
			Timeout:      ptypes.DurationProto(t.Timeout),
			Variables:    append([]string(nil), t.Vars...),
			VariableDeps: append([]string(nil), t.VarDeps...),
			SoftwareDeps: append([]string(nil), t.SoftwareDeps...),
			Bundle:       t.Bundle,
		},
	}
}

// WriteTestsAsProto exports test metadata in the protobuf format defined by infra.
func WriteTestsAsProto(w io.Writer, ts []*TestInstance) error {
	var result api.TestCaseMetadataList
	for _, src := range ts {
		result.Values = append(result.Values, src.Proto())
	}
	d, err := proto.Marshal(&result)
	if err != nil {
		return errors.Wrap(err, "Failed to marshalize the proto")
	}
	_, err = w.Write(d)
	return err
}

// RelativeDataDir returns the path to the directory in which data files for tests in pkg
// will be located, relative to the top-level directory containing data files.
func RelativeDataDir(pkg string) string {
	return filepath.Join(pkg, testDataSubdir)
}
