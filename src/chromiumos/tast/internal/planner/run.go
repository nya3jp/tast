// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"sync"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/dep"
	"chromiumos/tast/internal/devserver"
	"chromiumos/tast/internal/extdata"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/timing"
)

const (
	preTestTimeout  = 3 * time.Minute // timeout for RuntimeConfig.TestHook
	postTestTimeout = 3 * time.Minute // timeout for a closure returned by RuntimeConfig.TestHook
)

// DownloadMode specifies a strategy to download external data files.
type DownloadMode int

const (
	// DownloadBatch specifies that the planner downloads external data files
	// in batch before running tests.
	DownloadBatch DownloadMode = iota
	// DownloadLazy specifies that the planner download external data files
	// as needed between tests.
	DownloadLazy
)

// Config contains details about how the planner should run tests.
type Config struct {
	// DataDir is the path to the base directory containing test data files.
	DataDir string
	// OutDir is the path to the base directory under which tests should write output files.
	OutDir string
	// Vars contains names and values of runtime variables used to pass out-of-band data to tests.
	Vars map[string]string
	// Features contains software/hardware features the DUT has.
	Features dep.Features
	// Devservers contains URLs of devservers that can be used to download files.
	Devservers []string
	// TLWServer is the address of TLW server
	TLWServer string
	// DUTName is the name given by the infra scheduler
	DUTName string
	// BuildArtifactsURL is the URL of Google Cloud Storage directory, ending with a slash,
	// containing build artifacts for the current Chrome OS image.
	BuildArtifactsURL string
	// RemoteData contains information relevant to remote tests.
	// It is nil for local tests.
	RemoteData *testing.RemoteData
	// TestHook is run before TestInstance.Func (and TestInstance.Pre.Prepare, when applicable) if non-nil.
	// The returned closure is executed after a test if not nil.
	TestHook func(context.Context, *testing.TestHookState) func(context.Context, *testing.TestHookState)
	// DownloadMode specifies a strategy to download external data files.
	DownloadMode DownloadMode
	// Fixtures is a map from a fixture name to its metadata.
	Fixtures map[string]*testing.Fixture
	// StartFixtureName is a name of a fixture to start test execution.
	// Tests requested to run should depend on the start fixture directly or
	// indirectly.
	// Since a start fixture is treated specially (e.g. no output directory is
	// created), metadata of a start fixture must not be contained in
	// Config.Fixtures. Instead, StartFixtureImpl gives an implementation of
	// a start fixture.
	StartFixtureName string
	// StartFixtureImpl gives an implementation of a start fixture.
	// If it is nil, a default stub implementation is used.
	StartFixtureImpl testing.FixtureImpl
}

// RunTests runs a set of tests, writing outputs to out.
//
// RunTests is responsible for building an efficient plan to run the given tests.
// Therefore the order of tests in the argument is ignored; it just specifies
// a set of tests to run.
//
// RunTests runs tests on goroutines. If a test does not finish after reaching
// its timeout, this function returns with an error without waiting for its finish.
func RunTests(ctx context.Context, tests []*testing.TestInstance, out OutputStream, pcfg *Config) error {
	plan, err := buildPlan(tests, pcfg)
	if err != nil {
		return err
	}
	return plan.run(ctx, out)
}

// plan holds a top-level plan of test execution.
type plan struct {
	skips    []*skippedTest
	fixtPlan *fixtPlan
	prePlans []*prePlan
	pcfg     *Config
}

type skippedTest struct {
	test   *testing.TestInstance
	result *testing.ShouldRunResult
}

func buildPlan(tests []*testing.TestInstance, pcfg *Config) (*plan, error) {
	var runs []*testing.TestInstance
	var skips []*skippedTest
	for _, t := range tests {
		r := t.ShouldRun(&pcfg.Features)
		if r.OK() {
			runs = append(runs, t)
		} else {
			skips = append(skips, &skippedTest{test: t, result: r})
		}
	}
	sort.Slice(skips, func(i, j int) bool {
		return skips[i].test.Name < skips[j].test.Name
	})

	var fixtTests []*testing.TestInstance
	preMap := make(map[string][]*testing.TestInstance)
	for _, t := range runs {
		if t.Pre != nil {
			preName := t.Pre.String()
			preMap[preName] = append(preMap[preName], t)
		} else {
			fixtTests = append(fixtTests, t)
		}
	}

	preNames := make([]string, 0, len(preMap))
	for preName := range preMap {
		preNames = append(preNames, preName)
	}
	sort.Strings(preNames)

	prePlans := make([]*prePlan, len(preNames))
	for i, preName := range preNames {
		prePlans[i] = buildPrePlan(preMap[preName], pcfg)
	}

	fixtPlan, err := buildFixtPlan(fixtTests, pcfg)
	if err != nil {
		return nil, err
	}
	return &plan{skips, fixtPlan, prePlans, pcfg}, nil
}

func (p *plan) run(ctx context.Context, out OutputStream) error {
	dl, err := newDownloader(ctx, p.pcfg)
	if err != nil {
		return errors.Wrap(err, "failed to create new downloader")
	}
	defer dl.TearDown()
	dl.BeforeRun(ctx, p.testsToRun())

	for _, s := range p.skips {
		tout := newEntityOutputStream(out, s.test.EntityInfo())
		reportSkippedTest(tout, s.result)
	}

	if err := p.fixtPlan.run(ctx, out, dl); err != nil {
		return err
	}

	for _, pp := range p.prePlans {
		if err := pp.run(ctx, out, dl); err != nil {
			return err
		}
	}
	return nil
}

func (p *plan) testsToRun() []*testing.TestInstance {
	tests := p.fixtPlan.testsToRun()
	for _, pp := range p.prePlans {
		tests = append(tests, pp.testsToRun()...)
	}
	return tests
}

// fixtTree represents a fixture tree.
// At the beginning of fixture plan execution, a clone of a fixture tree is
// created, and finished tests are removed from the tree as we execute the plan
// to remember which tests are still to be run.
// A fixture tree is considered empty if it contains no test. An empty fixture
// tree must not appear as a subtree of another fixture tree so that we can
// check if a fixture tree is empty in O(1).
type fixtTree struct {
	fixt *testing.Fixture

	// Following fields are updated as we execute a plan.
	tests    []*testing.TestInstance
	children []*fixtTree
}

// Empty returns if a tree has no test.
// An empty fixture tree must not appear as a subtree of another fixture tree
// so that we can check if a fixture tree is empty in O(1).
func (t *fixtTree) Empty() bool {
	return len(t.tests) == 0 && len(t.children) == 0
}

// Clone returns a deep copy of fixtTree.
func (t *fixtTree) Clone() *fixtTree {
	children := make([]*fixtTree, len(t.children))
	for i, child := range t.children {
		children[i] = child.Clone()
	}
	return &fixtTree{
		fixt:     t.fixt,
		tests:    append([]*testing.TestInstance(nil), t.tests...),
		children: children,
	}
}

// fixtPlan holds an execution plan of fixture-ready tests.
type fixtPlan struct {
	pcfg *Config
	tree *fixtTree // original fixture tree; must not be modified
}

// buildFixtPlan builds an execution plan of fixture-ready tests. Tests passed
// to this function must not depend on preconditions.
func buildFixtPlan(tests []*testing.TestInstance, pcfg *Config) (*fixtPlan, error) {
	// Build a graph of fixtures relevant to the given tests.
	graph := make(map[string][]string) // fixture name to its child names
	seen := make(map[string]struct{})  // set of fixture names seen so far
	for _, t := range tests {
		cur := t.Fixture
		for cur != pcfg.StartFixtureName {
			if cur == "" {
				return nil, fmt.Errorf("cannot run test %q because it does not depend on start fixture %q", t.Name, pcfg.StartFixtureName)
			}
			if _, ok := seen[cur]; ok {
				break
			}
			seen[cur] = struct{}{}
			f, ok := pcfg.Fixtures[cur]
			if !ok {
				return nil, fmt.Errorf("fixture %q not found", cur)
			}
			graph[f.Parent] = append(graph[f.Parent], cur)
			cur = f.Parent
		}
	}
	for _, children := range graph {
		sort.Strings(children)
	}

	// Build a map from fixture names to tests.
	testMap := make(map[string][]*testing.TestInstance)
	for _, t := range tests {
		testMap[t.Fixture] = append(testMap[t.Fixture], t)
	}
	for _, ts := range testMap {
		sort.Slice(ts, func(i, j int) bool {
			return ts[i].Name < ts[j].Name
		})
	}

	// Traverse the graph to build a fixture tree.
	var traverse func(cur string) *fixtTree
	traverse = func(cur string) *fixtTree {
		var children []*fixtTree
		for _, child := range graph[cur] {
			children = append(children, traverse(child))
		}
		return &fixtTree{
			fixt:     pcfg.Fixtures[cur],
			tests:    testMap[cur],
			children: children,
		}
	}
	tree := traverse(pcfg.StartFixtureName)

	// Metadata of a start fixture should be unavailable in the registry.
	if tree.fixt != nil {
		return nil, fmt.Errorf("BUG: metadata of start fixture %q is unexpectedly available", pcfg.StartFixtureName)
	}
	impl := pcfg.StartFixtureImpl
	if impl == nil {
		impl = &stubFixture{}
	}
	tree.fixt = &testing.Fixture{
		// Do not set Name of a start fixture. entityOutputStream do not emit
		// EntityStart/EntityEnd for unnamed entities.
		Impl: impl,
		// TODO(crbug.com/1035940): Set timeouts of a start fixture.
	}

	return &fixtPlan{pcfg: pcfg, tree: tree}, nil
}

func (p *fixtPlan) run(ctx context.Context, out OutputStream, dl *downloader) error {
	tree := p.tree.Clone()
	stack := newFixtureStack(p.pcfg, out)
	return runFixtTree(ctx, tree, stack, p.pcfg, out, dl)
}

func (p *fixtPlan) testsToRun() []*testing.TestInstance {
	var tests []*testing.TestInstance

	var traverse func(tree *fixtTree)
	traverse = func(tree *fixtTree) {
		tests = append(tests, tree.tests...)
		for _, subtree := range tree.children {
			traverse(subtree)
		}
	}
	traverse(p.tree)

	return tests
}

// runFixtTree runs tests in a fixture tree.
// tree is modified as tests are run.
func runFixtTree(ctx context.Context, tree *fixtTree, stack *fixtureStack, pcfg *Config, out OutputStream, dl *downloader) error {
	// Note about invariants:
	// On entering this function, if the fixture stack is green, it is clean.
	// Thus we don't need to reset fixtures before running a next test.
	// On returning from this function, if the fixture stack was green and the
	// fixture tree was non-empty on entering this function, the stack is dirty.
	for !tree.Empty() {
		if err := func() error {
			// Create a fixture-scoped context.
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			// Push a fixture to the stack. This will call SetUp if the fixture stack is green.
			if err := stack.Push(ctx, tree.fixt); err != nil {
				return err
			}
			// Do not defer stack.Pop call here. It is correct to not call TearDown when
			// returning an error because it happens only when the timeout is ignored.

			// Run direct child tests first.
			for stack.Status() != statusYellow && len(tree.tests) > 0 {
				t := tree.tests[0]
				tree.tests = tree.tests[1:]
				tout := newEntityOutputStream(out, t.EntityInfo())
				if err := runTest(ctx, t, tout, pcfg, &preConfig{}, stack, dl); err != nil {
					return err
				}
				if !tree.Empty() {
					if err := stack.Reset(ctx); err != nil {
						return err
					}
				}
			}

			// Run child fixtures recursively.
			for stack.Status() != statusYellow && len(tree.children) > 0 {
				subtree := tree.children[0]
				if err := runFixtTree(ctx, subtree, stack, pcfg, out, dl); err != nil {
					return err
				}
				// It is possible that a recursive call of runFixtTree aborted in middle of
				// execution due to reset failures. Remove the subtree only when it is empty.
				if subtree.Empty() {
					tree.children = tree.children[1:]
				}
				if stack.Status() != statusYellow && !tree.Empty() {
					if err := stack.Reset(ctx); err != nil {
						return err
					}
				}
			}

			// Pop the fixture from the stack. This will call TearDown if it is not red.
			if err := stack.Pop(ctx); err != nil {
				return err
			}
			return nil
		}(); err != nil {
			return err
		}
	}
	return nil
}

// prePlan holds execution plan of tests using the same precondition.
type prePlan struct {
	pre   testing.Precondition
	tests []*testing.TestInstance
	pcfg  *Config
}

func buildPrePlan(tests []*testing.TestInstance, pcfg *Config) *prePlan {
	sort.Slice(tests, func(i, j int) bool {
		return tests[i].Name < tests[j].Name
	})
	return &prePlan{tests[0].Pre, tests, pcfg}
}

func (p *prePlan) run(ctx context.Context, out OutputStream, dl *downloader) error {
	// Create a precondition-scoped context.
	ec := &testing.CurrentEntity{
		// OutDir is not available for a precondition-scoped context.
		HasSoftwareDeps: true,
		SoftwareDeps:    append([]string(nil), p.tests[0].SoftwareDeps...),
		ServiceDeps:     append([]string(nil), p.tests[0].ServiceDeps...),
	}
	plog := newPreLogger(out)
	pctx, cancel := context.WithCancel(testing.NewContext(ctx, ec, plog.Log))
	defer cancel()

	// Create an empty fixture stack. Tests using preconditions can't depend on
	// fixtures.
	stack := newFixtureStack(p.pcfg, out)

	for i, t := range p.tests {
		ti := t.EntityInfo()
		plog.SetCurrentTest(ti)
		tout := newEntityOutputStream(out, ti)
		precfg := &preConfig{
			ctx:   pctx,
			close: p.pre != nil && i == len(p.tests)-1,
		}
		if err := runTest(ctx, t, tout, p.pcfg, precfg, stack, dl); err != nil {
			return err
		}
		if i < len(p.tests)-1 {
			if err := stack.Reset(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *prePlan) testsToRun() []*testing.TestInstance {
	return append([]*testing.TestInstance(nil), p.tests...)
}

// preLogger is a logger behind precondition-scoped contexts. It emits
// precondition logs to OutputStream just as if they are emitted by a currently
// running test. Call SetCurrentTest to set a current test.
type preLogger struct {
	out OutputStream

	mu sync.Mutex
	ti *testing.EntityInfo
}

func newPreLogger(out OutputStream) *preLogger {
	return &preLogger{out: out}
}

// Log emits a log message to OutputStream just as if it is emitted by the
// current test. SetCurrentTest must be called before calling this method.
func (l *preLogger) Log(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out.EntityLog(l.ti, msg)
}

// SetCurrentTest sets the current test.
func (l *preLogger) SetCurrentTest(ti *testing.EntityInfo) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ti = ti
}

// preConfig contains information needed to interact with a precondition for
// a single test.
type preConfig struct {
	// ctx is a context that lives as long as the precondition. It is available
	// to preconditions as testing.PreState.PreCtx.
	ctx context.Context
	// close is true if the test is the last test using the precondition and thus
	// it should be closed.
	close bool
}

// runTest runs a single test, writing outputs messages to tout.
//
// runTest runs a test on a goroutine. If a test does not finish after reaching
// its timeout, this function returns with an error without waiting for its finish.
func runTest(ctx context.Context, t *testing.TestInstance, tout *entityOutputStream, pcfg *Config, precfg *preConfig, stack *fixtureStack, dl *downloader) error {
	fixtCtx := ctx

	// Attach a log that the test can use to report timing events.
	timingLog := timing.NewLog()
	ctx = timing.NewContext(ctx, timingLog)

	outDir, err := createEntityOutDir(pcfg.OutDir, t.Name)
	if err != nil {
		return err
	}

	tout.Start(outDir)
	defer tout.End(nil, timingLog)

	dl.BeforeTest(ctx, t)

	if err := stack.MarkDirty(); err != nil {
		return err
	}

	switch stack.Status() {
	case statusGreen:
	case statusRed:
		msg := fmt.Sprintf("[Fixture failure] %s: Setup failed", stack.RedFixtureName())
		tout.Error(testing.NewError(nil, msg, msg, 0))
		return nil
	case statusYellow:
		return errors.New("BUG: Cannot run a test on a yellow fixture stack")
	}

	rcfg := &testing.RuntimeConfig{
		DataDir:      filepath.Join(pcfg.DataDir, testing.RelativeDataDir(t.Pkg)),
		OutDir:       outDir,
		Vars:         pcfg.Vars,
		CloudStorage: testing.NewCloudStorage(pcfg.Devservers, pcfg.TLWServer, pcfg.DUTName),
		RemoteData:   pcfg.RemoteData,
		FixtCtx:      fixtCtx,
		FixtValue:    stack.Val(),
		PreCtx:       precfg.ctx,
		Purgeable:    dl.Purgeable(),
	}
	root := testing.NewTestEntityRoot(t, rcfg, tout)

	if err := runTestWithRoot(ctx, t, root, pcfg, precfg); err != nil {
		// If runTestWithRoot reported that the test didn't finish, print diagnostic messages.
		msg := fmt.Sprintf("%v (see log for goroutine dump)", err)
		tout.Error(testing.NewError(nil, msg, msg, 0))
		dumpGoroutines(tout)
		return err
	}

	return nil
}

// downloader encapsulates the logic to download external data files.
type downloader struct {
	pcfg *Config
	cl   devserver.Client

	purgeable []string
}

func newDownloader(ctx context.Context, pcfg *Config) (*downloader, error) {
	cl, err := devserver.NewClient(ctx, pcfg.Devservers, pcfg.TLWServer, pcfg.DUTName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create new client [devservers=%v, TLWServer=%s]",
			pcfg.Devservers, pcfg.TLWServer)
	}
	return &downloader{
		pcfg: pcfg,
		cl:   cl,
	}, nil
}

// TearDown must be called when downloader is destructed.
func (d *downloader) TearDown() error {
	return d.cl.TearDown()
}

// BeforeRun must be called before running a set of tests. It downloads external
// data files if Config.DownloadMode is DownloadBatch.
func (d *downloader) BeforeRun(ctx context.Context, tests []*testing.TestInstance) {
	if d.pcfg.DownloadMode == DownloadBatch {
		d.download(ctx, tests)
	}
}

// BeforeTest must be called before running each test. It downloads external
// data files if Config.DownloadMode is DownloadLazy.
func (d *downloader) BeforeTest(ctx context.Context, test *testing.TestInstance) {
	if d.pcfg.DownloadMode == DownloadLazy {
		// TODO(crbug.com/1106218): Make sure this approach is scalable.
		// Recomputing purgeable on each test costs O(|purgeable| * |tests|) overall.
		d.download(ctx, []*testing.TestInstance{test})
	}
}

// Purgeable returns a list of cached external data files that can be deleted without
// disrupting the test execution.
func (d *downloader) Purgeable() []string {
	return append([]string(nil), d.purgeable...)
}

func (d *downloader) download(ctx context.Context, tests []*testing.TestInstance) {
	d.purgeable = extdata.Ensure(ctx, d.pcfg.DataDir, d.pcfg.BuildArtifactsURL, tests, d.cl)
}

func createEntityOutDir(baseDir, name string) (string, error) {
	// baseDir can be blank for unit tests.
	if baseDir == "" {
		return "", nil
	}

	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", err
	}

	var outDir string
	// First try to make a fixed-name directory. This allows unit tests to be deterministic.
	if err := os.Mkdir(filepath.Join(baseDir, name), 0755); err == nil {
		outDir = filepath.Join(baseDir, name)
	} else if os.IsExist(err) {
		// The directory already exists. Use ioutil.TempDir to create a randomly named one.
		var err error
		outDir, err = ioutil.TempDir(baseDir, name+".")
		if err != nil {
			return "", err
		}
	} else {
		return "", err
	}

	// Make the directory world-writable so that tests can create files as other users,
	// and set the sticky bit to prevent users from deleting other users' files.
	// (The mode passed to os.MkdirAll is modified by umask, so we need an explicit chmod.)
	if err := os.Chmod(outDir, 0777|os.ModeSticky); err != nil {
		return "", err
	}
	return outDir, nil
}

// runTestWithRoot runs a test with TestEntityRoot.
//
// The time allotted to the test is generally the sum of t.Timeout and t.ExitTimeout, but
// additional time may be allotted for preconditions and pre/post-test hooks.
func runTestWithRoot(ctx context.Context, t *testing.TestInstance, root *testing.TestEntityRoot, pcfg *Config, precfg *preConfig) error {
	// codeName is included in error messages if the user code ignores the timeout.
	// For compatibility, the same fixed name is used for tests, preconditions and test hooks.
	const codeName = "Test"

	var postTestFunc func(ctx context.Context, s *testing.TestHookState)

	ctx = root.NewContext(ctx)
	testState := root.NewTestState()

	// First, perform setup and run the pre-test function.
	if err := safeCall(ctx, codeName, preTestTimeout, defaultGracePeriod, errorOnPanic(testState), func(ctx context.Context) {
		// The test bundle is responsible for ensuring t.Timeout is nonzero before calling Run,
		// but we call s.Fatal instead of panicking since it's arguably nicer to report individual
		// test failures instead of aborting the entire run.
		if t.Timeout <= 0 {
			testState.Fatal("Invalid timeout ", t.Timeout)
		}

		// Make sure all required data files exist.
		for _, fn := range t.Data {
			fp := testState.DataPath(fn)
			if _, err := os.Stat(fp); err == nil {
				continue
			}
			ep := fp + testing.ExternalErrorSuffix
			if data, err := ioutil.ReadFile(ep); err == nil {
				testState.Errorf("Required data file %s missing: %s", fn, string(data))
			} else {
				testState.Errorf("Required data file %s missing", fn)
			}
		}
		if testState.HasError() {
			return
		}

		// In remote tests, reconnect to the DUT if needed.
		if pcfg.RemoteData != nil {
			dt := testState.DUT()
			if !dt.Connected(ctx) {
				testState.Log("Reconnecting to DUT")
				if err := dt.Connect(ctx); err != nil {
					testState.Fatal("Failed to reconnect to DUT: ", err)
				}
			}
		}

		if pcfg.TestHook != nil {
			postTestFunc = pcfg.TestHook(ctx, root.NewTestHookState())
		}
	}); err != nil {
		return err
	}

	// TODO(crbug.com/1035940): Support fixture pre-test hooks.

	// Prepare the test's precondition (if any) if setup was successful.
	if !root.HasError() && t.Pre != nil {
		preState := root.NewPreState()
		if err := safeCall(ctx, codeName, t.Pre.Timeout(), defaultGracePeriod, errorOnPanic(preState), func(ctx context.Context) {
			preState.Logf("Preparing precondition %q", t.Pre)
			root.SetPreValue(t.Pre.Prepare(ctx, preState))
		}); err != nil {
			return err
		}
	}

	// Next, run the test function itself if no errors have been reported so far.
	if !root.HasError() {
		if err := safeCall(ctx, codeName, t.Timeout, timeoutOrDefault(t.ExitTimeout, defaultGracePeriod), errorOnPanic(testState), func(ctx context.Context) {
			t.Func(ctx, testState)
		}); err != nil {
			return err
		}
	}

	// If this is the final test using this precondition, close it
	// (even if setup, t.Pre.Prepare, or t.Func failed).
	if precfg.close {
		preState := root.NewPreState()
		if err := safeCall(ctx, codeName, t.Pre.Timeout(), defaultGracePeriod, errorOnPanic(preState), func(ctx context.Context) {
			preState.Logf("Closing precondition %q", t.Pre.String())
			t.Pre.Close(ctx, preState)
		}); err != nil {
			return err
		}
	}

	// TODO(crbug.com/1035940): Support fixture post-test hooks.

	// Finally, run the post-test functions unconditionally.
	if postTestFunc != nil {
		if err := safeCall(ctx, codeName, postTestTimeout, defaultGracePeriod, errorOnPanic(testState), func(ctx context.Context) {
			postTestFunc(ctx, root.NewTestHookState())
		}); err != nil {
			return err
		}
	}

	return nil
}

// timeoutOrDefault returns timeout if positive or def otherwise.
func timeoutOrDefault(timeout, def time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	return def
}

// reportSkippedTest is called instead of runTest for a test that is skipped due to
// having unsatisfied dependencies.
func reportSkippedTest(tout *entityOutputStream, result *testing.ShouldRunResult) {
	tout.Start("")
	for _, msg := range result.Errors {
		_, fn, ln, _ := runtime.Caller(0)
		tout.Error(&testing.Error{
			Reason: msg,
			File:   fn,
			Line:   ln,
		})
	}
	tout.End(result.SkipReasons, nil)
}

// dumpGoroutines dumps all goroutines to tout.
func dumpGoroutines(tout *entityOutputStream) {
	tout.Log("Dumping all goroutines")
	if err := func() error {
		p := pprof.Lookup("goroutine")
		if p == nil {
			return errors.New("goroutine pprof not found")
		}
		var buf bytes.Buffer
		if err := p.WriteTo(&buf, 2); err != nil {
			return err
		}
		sc := bufio.NewScanner(&buf)
		for sc.Scan() {
			tout.Log(sc.Text())
		}
		return sc.Err()
	}(); err != nil {
		tout.Error(&testing.Error{
			Reason: fmt.Sprintf("Failed to dump goroutines: %v", err),
		})
	}
}

// stubFixture is a stub implementation of testing.FixtureImpl.
type stubFixture struct{}

func (f *stubFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} { return nil }
func (f *stubFixture) Reset(ctx context.Context) error                             { return nil }
func (f *stubFixture) PreTest(ctx context.Context, s *testing.FixtTestState)       {}
func (f *stubFixture) PostTest(ctx context.Context, s *testing.FixtTestState)      {}
func (f *stubFixture) TearDown(ctx context.Context, s *testing.FixtState)          {}
