// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"sync"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/planner/internal/entity"
	"chromiumos/tast/internal/planner/internal/fixture"
	"chromiumos/tast/internal/planner/internal/output"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/timing"
	"chromiumos/tast/internal/usercode"
)

// OutputStream is an interface to report streamed outputs of multiple entity runs.
type OutputStream = output.Stream

// FixtureStack maintains a stack of fixtures and their states.
type FixtureStack = fixture.InternalStack

// NewFixtureStack creates a new empty fixture stack.
func NewFixtureStack(cfg *Config, out OutputStream) *FixtureStack {
	return fixture.NewInternalStack(cfg.FixtureConfig(), out)
}

const (
	preTestTimeout  = 3 * time.Minute // timeout for RuntimeConfig.TestHook
	postTestTimeout = 3 * time.Minute // timeout for a closure returned by RuntimeConfig.TestHook

	// DefaultGracePeriod is default recommended grace period for SafeCall.
	DefaultGracePeriod = 30 * time.Second
)

// Config contains details about how the planner should run tests.
type Config struct {
	// Dirs holds several directory paths important for running tests.
	Dirs *protocol.RunDirectories

	// Features contains software/hardware features the DUT has, and runtime variables.
	Features *protocol.Features

	// ServiceConfig contains configurations of external services available to
	// Tast framework and Tast tests.
	Service *protocol.ServiceConfig

	// DataFileConfig contains configurations about data files.
	DataFile *protocol.DataFileConfig

	// RemoteData contains information relevant to remote tests.
	// It is nil for local tests.
	RemoteData *testing.RemoteData
	// TestHook is run before TestInstance.Func (and TestInstance.Pre.Prepare, when applicable) if non-nil.
	// The returned closure is executed after a test if not nil.
	TestHook func(context.Context, *testing.TestHookState) func(context.Context, *testing.TestHookState)

	// BeforeDownload specifies a function called before downloading external data files.
	// It is ignored if it is nil.
	BeforeDownload func(context.Context)
	// Tests is a map from a test name to its metadata.
	Tests map[string]*testing.TestInstance
	// Fixtures is a map from a fixture name to its metadata.
	Fixtures map[string]*testing.FixtureInstance
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
	// CustomGracePeriod specifies custom grace period after entity timeout.
	// If nil reasonable default will be used. Config.GracePeriod() returns
	// the grace period to use. This field exists for unit testing.
	CustomGracePeriod *time.Duration

	// ExternalTarget represents configs for running an external bundle from
	// current bundle. (i.e. local bundle from remote bundle).
	ExternalTarget *ExternalTarget
}

// GracePeriod returns grace period after entity timeout.
func (c *Config) GracePeriod() time.Duration {
	if c.CustomGracePeriod != nil {
		return *c.CustomGracePeriod
	}
	return DefaultGracePeriod
}

// FixtureConfig returns a fixture config derived from c.
func (c *Config) FixtureConfig() *fixture.Config {
	return &fixture.Config{
		DataDir:           c.Dirs.GetDataDir(),
		OutDir:            c.Dirs.GetOutDir(),
		Vars:              c.Features.GetInfra().GetVars(),
		Service:           c.Service,
		BuildArtifactsURL: c.DataFile.GetBuildArtifactsUrl(),
		RemoteData:        c.RemoteData,
		StartFixtureName:  c.StartFixtureName,
		GracePeriod:       c.GracePeriod(),
	}
}

// RunTestsLegacy runs a set of tests, writing outputs to out.
//
// RunTestsLegacy is responsible for building an efficient plan to run the given tests.
// Therefore the order of tests in the argument is ignored; it just specifies
// a set of tests to run.
//
// RunTestsLegacy runs tests on goroutines. If a test does not finish after reaching
// its timeout, this function returns with an error without waiting for its finish.
func RunTestsLegacy(ctx context.Context, tests []*testing.TestInstance, out OutputStream, pcfg *Config) error {
	if pcfg.Tests != nil {
		return fmt.Errorf("BUG: RunTestsLegacy pcfg.Tests = %v, want nil", pcfg.Tests)
	}
	// HACK: modify Tests field. This code should soon go away along with the
	// removal of this function.
	defer func() {
		pcfg.Tests = nil
	}()
	pcfg.Tests = make(map[string]*testing.TestInstance)
	for _, t := range tests {
		pcfg.Tests[t.Name] = t
	}
	ts := make([]*protocol.ResolvedEntity, len(tests))
	for i, t := range tests {
		ts[i] = &protocol.ResolvedEntity{
			Hops:   0,
			Entity: t.EntityProto(),
		}
	}
	return RunTests(ctx, ts, out, pcfg)
}

// RunTests runs a set of tests, writing outputs to out.
//
// RunTests is responsible for building an efficient plan to run the given tests.
// Therefore the order of tests in the argument is ignored; it just specifies
// a set of tests to run.
//
// RunTests runs tests on goroutines. If a test does not finish after reaching
// its timeout, this function returns with an error without waiting for its finish.
func RunTests(ctx context.Context, tests []*protocol.ResolvedEntity, out OutputStream, pcfg *Config) error {
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
	test    *testing.TestInstance
	reasons []string
	err     error
}

func buildPlan(tests []*protocol.ResolvedEntity, pcfg *Config) (*plan, error) {
	var testsWithFixture []*protocol.ResolvedEntity
	preMap := make(map[string][]*testing.TestInstance)
	var skips []*skippedTest
	for _, t := range tests {
		if t.GetHops() > 0 {
			testsWithFixture = append(testsWithFixture, t)
			continue
		}
		ti, ok := pcfg.Tests[t.GetEntity().GetName()]
		if !ok {
			return nil, fmt.Errorf("BUG: test %v does not exist", t.GetEntity().GetName())
		}
		reasons, err := ti.Deps().Check(pcfg.Features)
		if err != nil || len(reasons) > 0 {
			skips = append(skips, &skippedTest{test: ti, reasons: reasons, err: err})
			continue
		}
		if ti.Pre != nil {
			preName := ti.Pre.String()
			preMap[preName] = append(preMap[preName], ti)
			continue
		}
		// A test which is not skipped nor depending on a precondition is
		// fixture-ready, possibly depending on an empty fixture.
		testsWithFixture = append(testsWithFixture, t)
	}
	sort.Slice(skips, func(i, j int) bool {
		return skips[i].test.Name < skips[j].test.Name
	})

	preNames := make([]string, 0, len(preMap))
	for preName := range preMap {
		preNames = append(preNames, preName)
	}
	sort.Strings(preNames)

	prePlans := make([]*prePlan, len(preNames))
	for i, preName := range preNames {
		prePlans[i] = buildPrePlan(preMap[preName], pcfg)
	}

	fixtPlan, err := buildFixtPlan(testsWithFixture, pcfg)
	if err != nil {
		return nil, err
	}
	return &plan{skips, fixtPlan, prePlans, pcfg}, nil
}

func (p *plan) run(ctx context.Context, out output.Stream) error {
	dl, err := newDownloader(ctx, p.pcfg)
	if err != nil {
		return errors.Wrap(err, "failed to create new downloader")
	}
	defer dl.TearDown()
	dl.BeforeRun(ctx, p.entitiesToRun())

	for _, s := range p.skips {
		tout := output.NewEntityStream(out, s.test.EntityProto())
		reportSkippedTest(tout, s.reasons, s.err)
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

func (p *plan) entitiesToRun() []*protocol.Entity {
	var res = p.fixtPlan.entitiesToRun()
	for _, pp := range p.prePlans {
		for _, t := range pp.testsToRun() {
			res = append(res, t.EntityProto())
		}
	}
	return res
}

// fixtTree represents a fixture tree.
// At the beginning of fixture plan execution, a clone of a fixture tree is
// created, and finished tests are removed from the tree as we execute the plan
// to remember which tests are still to be run.
// A fixture tree is considered empty if it contains no test. An empty fixture
// tree must not appear as a subtree of another fixture tree so that we can
// check if a fixture tree is empty in O(1).
type fixtTree struct {
	fixt *testing.FixtureInstance

	// Following fields are updated as we execute a plan.
	tests         []*testing.TestInstance
	externalTests []string
	children      []*fixtTree
}

// Empty returns if a tree has no test.
// An empty fixture tree must not appear as a subtree of another fixture tree
// so that we can check if a fixture tree is empty in O(1).
func (t *fixtTree) Empty() bool {
	return len(t.tests) == 0 && len(t.externalTests) == 0 && len(t.children) == 0
}

// Clone returns a deep copy of fixtTree.
func (t *fixtTree) Clone() *fixtTree {
	children := make([]*fixtTree, len(t.children))
	for i, child := range t.children {
		children[i] = child.Clone()
	}
	return &fixtTree{
		fixt:          t.fixt,
		tests:         append([]*testing.TestInstance(nil), t.tests...),
		externalTests: append([]string(nil), t.externalTests...),
		children:      children,
	}
}

// orphanTest represents a test that depends on a missing fixture directly or
// indirectly.
type orphanTest struct {
	test            *protocol.ResolvedEntity
	missingFixtName string
}

// fixtPlan holds an execution plan of fixture-ready tests.
type fixtPlan struct {
	pcfg    *Config
	tree    *fixtTree // original fixture tree; must not be modified
	orphans []*orphanTest
}

// buildFixtPlan builds an execution plan of fixture-ready tests. Tests passed
// to this function must not depend on preconditions.
func buildFixtPlan(tests []*protocol.ResolvedEntity, pcfg *Config) (*fixtPlan, error) {
	var orphans []*orphanTest
	testsToRun := make(map[string][]*testing.TestInstance) // keyed by fixture
	externalTestsToRun := make(map[string][]string)

	// Build a graph of fixtures relevant to the given tests.
	graph := make(map[string][]string) // fixture name to its children names
	added := make(map[string]struct{}) // set of fixtures added to graph as children
	var traverse func(string) (bool, string)
	traverse = func(fixture string) (rooted bool, missingFixtureName string) {
		if fixture == pcfg.StartFixtureName {
			return true, ""
		}
		if _, ok := added[fixture]; ok {
			return true, ""
		}
		f, ok := pcfg.Fixtures[fixture]
		if !ok {
			return false, fixture
		}
		rooted, missing := traverse(f.Parent)
		if rooted {
			added[fixture] = struct{}{}
			graph[f.Parent] = append(graph[f.Parent], fixture)
		}
		return rooted, missing
	}
	for _, t := range tests {
		f := fixtTreeParent(t)
		rooted, missing := traverse(f)
		if !rooted {
			orphans = append(orphans, &orphanTest{
				test:            t,
				missingFixtName: missing,
			})
		} else if t.Hops == 0 {
			// Existence of the test instance is already checked in buildPlan.
			testsToRun[f] = append(testsToRun[f], pcfg.Tests[t.GetEntity().GetName()])
		} else {
			externalTestsToRun[f] = append(externalTestsToRun[f], t.GetEntity().GetName())
		}
	}
	// Normalize
	sort.Slice(orphans, func(i, j int) bool {
		ei := orphans[i].test
		ej := orphans[j].test
		if ei.Hops != ej.Hops {
			return ei.Hops < ej.Hops
		}
		return ei.Entity.Name < ej.Entity.Name
	})
	for _, ts := range testsToRun {
		sort.Slice(ts, func(i, j int) bool {
			return ts[i].Name < ts[j].Name
		})
	}
	for _, ts := range externalTestsToRun {
		sort.Strings(ts)
	}
	for _, children := range graph {
		sort.Strings(children)
	}

	impl := pcfg.StartFixtureImpl
	if impl == nil {
		impl = &stubFixture{}
	}
	const infinite = 24 * time.Hour // a day is considered infinite
	rootFixture := &testing.FixtureInstance{
		// Do not set Name of a start fixture. output.EntityOutputStream do not
		// emit EntityStart/EntityEnd for unnamed entities.
		Impl: impl,
		// Set infinite timeouts to all lifecycle methods. In production, the
		// start fixture may communicate with the host machine to trigger remote
		// fixtures, which would take quite some time but timeouts are responsibly
		// handled by the host binary. In unit tests, we may set the custom grace
		// period to very small values (like a millisecond) to test the behavior
		// when user code ignore timeouts, where we need long timeouts to avoid
		// hitting timeouts in the start fixture.
		SetUpTimeout:    infinite,
		TearDownTimeout: infinite,
		ResetTimeout:    infinite,
		PreTestTimeout:  infinite,
		PostTestTimeout: infinite,
	}

	// Traverse the graph to build a fixture tree.
	var newTree func(name string) *fixtTree
	newTree = func(name string) *fixtTree {
		var f *testing.FixtureInstance
		if name == pcfg.StartFixtureName {
			f = rootFixture
		} else {
			f = pcfg.Fixtures[name]
		}
		var children []*fixtTree
		for _, c := range graph[name] {
			children = append(children, newTree(c))
		}
		return &fixtTree{
			fixt:          f,
			tests:         testsToRun[name],
			externalTests: externalTestsToRun[name],
			children:      children,
		}
	}
	tree := newTree(pcfg.StartFixtureName)
	return &fixtPlan{pcfg: pcfg, tree: tree, orphans: orphans}, nil
}

func fixtTreeParent(test *protocol.ResolvedEntity) string {
	if test.Hops > 0 {
		return test.StartFixtureName
	}
	return test.Entity.Fixture
}

func (p *fixtPlan) run(ctx context.Context, out output.Stream, dl *downloader) error {
	for _, o := range p.orphans {
		tout := output.NewEntityStream(out, o.test.GetEntity())
		reportOrphanTest(tout, o.missingFixtName)
	}

	tree := p.tree.Clone()
	internalStack := fixture.NewInternalStack(p.pcfg.FixtureConfig(), out)

	var stack *internalOrCombinedStack
	if p.pcfg.ExternalTarget != nil {
		externalStack, err := fixture.NewExternalStack(ctx, out)
		if err != nil {
			return err
		}
		combinedStack := fixture.NewCombinedStack(externalStack, internalStack)
		stack = &internalOrCombinedStack{combined: combinedStack}
	} else {
		// TODO(oka): Remove this code after migration for full remote fixtures.
		// After migration is fully finished, only CombinedStack should be used.
		stack = &internalOrCombinedStack{internal: internalStack}
	}

	return runFixtTree(ctx, tree, stack, p.pcfg, out, dl)
}

func (p *fixtPlan) entitiesToRun() []*protocol.Entity {
	var entities []*protocol.Entity

	for _, o := range p.orphans {
		entities = append(entities, o.test.GetEntity())
	}

	var traverse func(tree *fixtTree)
	traverse = func(tree *fixtTree) {
		if tree.fixt.Name != "" {
			entities = append(entities, tree.fixt.EntityProto())
		}
		for _, t := range tree.tests {
			entities = append(entities, t.EntityProto())
		}
		for _, subtree := range tree.children {
			traverse(subtree)
		}
	}
	traverse(p.tree)

	return entities
}

// runFixtTree runs tests in a fixture tree.
// tree is modified as tests are run.
func runFixtTree(ctx context.Context, tree *fixtTree, stack *internalOrCombinedStack, pcfg *Config, out output.Stream, dl *downloader) error {
	// Note about invariants:
	// On entering this function, if the fixture stack is green, it is clean.
	// Thus we don't need to reset fixtures before running a next test.
	// On returning from this function, if the fixture stack was green and the
	// fixture tree was non-empty on entering this function, the stack is dirty.
	for !tree.Empty() && stack.Status() != fixture.StatusYellow {
		if err := func() error {
			// Create a fixture-scoped context.
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			release := dl.BeforeEntity(ctx, tree.fixt.EntityProto())
			defer release()

			// Push a fixture to the stack. This will call SetUp if the fixture stack is green.
			if err := stack.Push(ctx, tree.fixt); err != nil {
				return err
			}
			// Do not defer stack.Pop call here. It is correct to not call TearDown when
			// returning an error because it happens only when the timeout is ignored.

			// Run direct child tests first.
			for stack.Status() != fixture.StatusYellow && len(tree.tests) > 0 {
				t := tree.tests[0]
				tree.tests = tree.tests[1:]
				tout := output.NewEntityStream(out, t.EntityProto())
				if err := runTest(ctx, t, tout, pcfg, &preConfig{}, stack, dl); err != nil {
					return err
				}
				if !tree.Empty() {
					if err := stack.Reset(ctx); err != nil {
						return err
					}
				}
			}
			// Run external tests then.
			for stack.Status() != fixture.StatusYellow && len(tree.externalTests) > 0 {
				unstarted, err := runExternalTests(ctx, tree.externalTests, stack.combined, pcfg, out)
				if err != nil {
					return err
				}
				if len(unstarted) == len(tree.externalTests) {
					return fmt.Errorf("BUG: runExternalTests succeeds but no external test has run")
				}
				tree.externalTests = unstarted
			}

			// Run child fixtures recursively.
			for stack.Status() != fixture.StatusYellow && len(tree.children) > 0 {
				subtree := tree.children[0]
				if err := runFixtTree(ctx, subtree, stack, pcfg, out, dl); err != nil {
					return err
				}
				// It is possible that a recursive call of runFixtTree aborted in middle of
				// execution due to reset failures. Remove the subtree only when it is empty.
				if subtree.Empty() {
					tree.children = tree.children[1:]
				}
				if stack.Status() != fixture.StatusYellow && !tree.Empty() {
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

func (p *prePlan) run(ctx context.Context, out output.Stream, dl *downloader) error {
	// Create a precondition-scoped context.
	ec := &testcontext.CurrentEntity{
		// OutDir is not available for a precondition-scoped context.
		HasSoftwareDeps: true,
		SoftwareDeps:    append([]string(nil), p.tests[0].SoftwareDeps...),
		ServiceDeps:     append([]string(nil), p.tests[0].ServiceDeps...),
		Labels:          append([]string(nil), p.tests[0].Labels...),
	}
	plog := newPreLogger(out)
	pctx, cancel := context.WithCancel(testing.NewContext(ctx, ec, logging.NewFuncSink(plog.Log)))
	defer cancel()

	// Create an empty fixture stack. Tests using preconditions can't depend on
	// fixture.
	internalStack := fixture.NewInternalStack(p.pcfg.FixtureConfig(), out)
	stack := &internalOrCombinedStack{internal: internalStack}

	for i, t := range p.tests {
		ti := t.EntityProto()
		plog.SetCurrentTest(ti)
		tout := output.NewEntityStream(out, ti)
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
// precondition logs to output.OutputStream just as if they are emitted by a currently
// running test. Call SetCurrentTest to set a current test.
type preLogger struct {
	out output.Stream

	mu sync.Mutex
	ti *protocol.Entity
}

func newPreLogger(out output.Stream) *preLogger {
	return &preLogger{out: out}
}

// Log emits a log message to output.OutputStream just as if it is emitted by the
// current test. SetCurrentTest must be called before calling this method.
func (l *preLogger) Log(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out.EntityLog(l.ti, msg)
}

// SetCurrentTest sets the current test.
func (l *preLogger) SetCurrentTest(ti *protocol.Entity) {
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
func runTest(ctx context.Context, t *testing.TestInstance, tout *output.EntityStream, pcfg *Config, precfg *preConfig, stack *internalOrCombinedStack, dl *downloader) error {
	fixtCtx := ctx

	// Attach a log that the test can use to report timing events.
	timingLog := timing.NewLog()
	ctx = timing.NewContext(ctx, timingLog)

	outDir, err := entity.CreateOutDir(pcfg.Dirs.GetOutDir(), t.Name)
	if err != nil {
		return err
	}

	tout.Start(outDir)
	defer tout.End(nil, timingLog)

	afterTest := dl.BeforeEntity(ctx, t.EntityProto())
	defer afterTest()

	if err := stack.MarkDirty(ctx); err != nil {
		return err
	}

	switch stack.Status() {
	case fixture.StatusGreen:
	case fixture.StatusRed:
		for _, e := range stack.Errors() {
			tout.Error(e)
		}
		return nil
	case fixture.StatusYellow:
		return errors.New("BUG: Cannot run a test on a yellow fixture stack")
	}

	tcfg := &testConfig{
		test:    t,
		outDir:  outDir,
		fixtCtx: fixtCtx,
		// TODO(crbug.com/1106218): Make sure this approach is scalable.
		// Recomputing purgeable on each test costs O(|purgeable| * |tests|) overall.
		purgeable: dl.m.Purgeable(),
	}
	if err := runTestWithConfig(ctx, tcfg, pcfg, stack, precfg, tout); err != nil {
		// If runTestWithRoot reported that the test didn't finish, print diagnostic messages.
		msg := fmt.Sprintf("%v (see log for goroutine dump)", err)
		tout.Error(testing.NewError(nil, msg, msg, 0))
		dumpGoroutines(tout)
		return err
	}

	return nil
}

type testConfig struct {
	test      *testing.TestInstance
	outDir    string
	fixtCtx   context.Context
	purgeable []string
}

// runTestWithConfig runs a test on the given configs.
//
// The time allotted to the test is generally the sum of t.Timeout and t.ExitTimeout, but
// additional time may be allotted for preconditions and pre/post-test hooks.
func runTestWithConfig(ctx context.Context, tcfg *testConfig, pcfg *Config, stack *internalOrCombinedStack, precfg *preConfig, out testing.OutputStream) error {
	// codeName is included in error messages if the user code ignores the timeout.
	// For compatibility, the same fixed name is used for tests, preconditions and test hooks.
	const codeName = "Test"

	var postTestFunc func(ctx context.Context, s *testing.TestHookState)

	condition := testing.NewEntityCondition()
	rcfg := &testing.RuntimeConfig{
		DataDir: filepath.Join(pcfg.Dirs.GetDataDir(), testing.RelativeDataDir(tcfg.test.Pkg)),
		OutDir:  tcfg.outDir,
		Vars:    pcfg.Features.GetInfra().GetVars(),
		CloudStorage: testing.NewCloudStorage(
			pcfg.Service.GetDevservers(),
			pcfg.Service.GetTlwServer(),
			pcfg.Service.GetTlwSelfName(),
			pcfg.Service.GetDutServer(),
			pcfg.DataFile.GetBuildArtifactsUrl(),
		),
		RemoteData: pcfg.RemoteData,
		FixtCtx:    tcfg.fixtCtx,
		FixtValue:  stack.Val(),
		PreCtx:     precfg.ctx,
		Purgeable:  tcfg.purgeable,
	}
	troot := testing.NewTestEntityRoot(tcfg.test, rcfg, out, condition)
	ctx = troot.NewContext(ctx)
	testState := troot.NewTestState()

	// First, perform setup and run the pre-test function.
	if err := usercode.SafeCall(ctx, codeName, preTestTimeout, pcfg.GracePeriod(), usercode.ErrorOnPanic(testState), func(ctx context.Context) {
		// The test bundle is responsible for ensuring t.Timeout is nonzero before calling Run,
		// but we call s.Fatal instead of panicking since it's arguably nicer to report individual
		// test failures instead of aborting the entire run.
		if tcfg.test.Timeout <= 0 {
			testState.Fatal("Invalid timeout ", tcfg.test.Timeout)
		}

		entity.PreCheck(tcfg.test.Data, testState)
		if testState.HasError() {
			return
		}

		// In remote tests, reconnect to the DUT if needed.
		if pcfg.RemoteData != nil {
			dt := testState.DUT()
			if !dt.Connected(ctx) {
				testState.Log("Reconnecting to DUT")
				if err := dt.Connect(ctx); err != nil {
					testState.Error(testing.TestDidNotRunMsg)
					testState.Fatal("Failed to reconnect to DUT: ", err)
				}
			}
		}

		if pcfg.TestHook != nil {
			postTestFunc = pcfg.TestHook(ctx, troot.NewTestHookState())
		}
	}); err != nil {
		return err
	}

	// Prepare the test's precondition (if any) if setup was successful.
	if !condition.HasError() && tcfg.test.Pre != nil {
		preState := troot.NewPreState()
		if err := usercode.SafeCall(ctx, codeName, tcfg.test.Pre.Timeout(), pcfg.GracePeriod(), usercode.ErrorOnPanic(preState), func(ctx context.Context) {
			preState.Logf("Preparing precondition %q", tcfg.test.Pre)
			troot.SetPreValue(tcfg.test.Pre.Prepare(ctx, preState))
		}); err != nil {
			return err
		}
	}

	if err := func() error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Run fixture pre-test hooks.
		postTest, err := stack.PreTest(ctx, tcfg.test.EntityProto(), tcfg.outDir, out, condition)
		if err != nil {
			return err
		}

		if !condition.HasError() {
			// Run the test function itself.
			if err := usercode.SafeCall(ctx, codeName, tcfg.test.Timeout, timeoutOrDefault(tcfg.test.ExitTimeout, pcfg.GracePeriod()), usercode.ErrorOnPanic(testState), func(ctx context.Context) {
				tcfg.test.Func(ctx, testState)
			}); err != nil {
				return err
			}
		}

		// Run fixture post-test hooks.
		if err := postTest(ctx); err != nil {
			return err
		}
		return nil
	}(); err != nil {
		return err
	}

	// If this is the final test using this precondition, close it
	// (even if setup, tcfg.test.Pre.Prepare, or tcfg.test.Func failed).
	if precfg.close {
		preState := troot.NewPreState()
		if err := usercode.SafeCall(ctx, codeName, tcfg.test.Pre.Timeout(), pcfg.GracePeriod(), usercode.ErrorOnPanic(preState), func(ctx context.Context) {
			preState.Logf("Closing precondition %q", tcfg.test.Pre.String())
			tcfg.test.Pre.Close(ctx, preState)
		}); err != nil {
			return err
		}
	}

	// Finally, run the post-test functions unconditionally.
	if postTestFunc != nil {
		if err := usercode.SafeCall(ctx, codeName, postTestTimeout, pcfg.GracePeriod(), usercode.ErrorOnPanic(testState), func(ctx context.Context) {
			postTestFunc(ctx, troot.NewTestHookState())
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

// reportOrphanTest is called instead of runTest for a test that depends on
// a missing fixture directly or indirectly.
func reportOrphanTest(tout *output.EntityStream, missingFixtName string) {
	tout.Start("")
	_, fn, ln, _ := runtime.Caller(0)
	tout.Error(&protocol.Error{
		Reason: fmt.Sprintf("Fixture %q not found", missingFixtName),
		Location: &protocol.ErrorLocation{
			File: fn,
			Line: int64(ln),
		},
	})
	tout.End(nil, timing.NewLog())
}

// reportSkippedTest is called instead of runTest for a test that is skipped due to
// having unsatisfied dependencies.
func reportSkippedTest(tout *output.EntityStream, reasons []string, err error) {
	tout.Start("")
	if err == nil {
		tout.End(reasons, timing.NewLog())
		return
	}

	_, fn, ln, _ := runtime.Caller(0)
	tout.Error(&protocol.Error{
		Reason: err.Error(),
		Location: &protocol.ErrorLocation{
			File: fn,
			Line: int64(ln),
		},
	})
	// Do not report a test as skipped if we encounter errors while checking
	// dependencies. There is ambiguity if a test is skipped while reporting
	// errors, and in the worst case, dependency check errors can be
	// silently discarded.
	tout.End(nil, timing.NewLog())
}

// dumpGoroutines dumps all goroutines to tout.
func dumpGoroutines(tout *output.EntityStream) {
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
		tout.Error(&protocol.Error{
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
