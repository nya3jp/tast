// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package planner

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"sync"
	"time"

	"chromiumos/tast/internal/dep"
	"chromiumos/tast/internal/devserver"
	"chromiumos/tast/internal/extdata"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/timing"
)

const (
	exitTimeout     = 30 * time.Second // extra time granted to test-related funcs to exit
	preTestTimeout  = 3 * time.Minute  // timeout for RuntimeConfig.TestHook
	postTestTimeout = 3 * time.Minute  // timeout for a closure returned by RuntimeConfig.TestHook
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
	dl := newDownloader(ctx, p.pcfg)
	dl.BeforeRun(ctx, p.testsToRun())

	for _, s := range p.skips {
		tout := newEntityOutputStream(out, s.test.EntityInfo())
		reportSkippedTest(tout, s.result)
	}

	st := newFixtureStack(p.pcfg)
	if err := p.fixtPlan.run(ctx, st, out, dl); err != nil {
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

// TODO
type fixtPlan struct {
	pcfg     *Config
	fixt     *testing.Fixture
	tests    []*testing.TestInstance
	children []*fixtPlan
}

// TODO
func buildFixtPlan(tests []*testing.TestInstance, pcfg *Config) (*fixtPlan, error) {
	// topFixt is the name of the implicit top-level fixture.
	const topFixt = ""

	// Build a fixture tree relevant to the given tests.
	tree := make(map[string][]string) // fixture name to its child names
	seen := make(map[string]struct{}) // set of fixture names seen so far
	for _, t := range tests {
		cur := t.Fixture
		for cur != topFixt {
			if _, ok := seen[cur]; ok {
				break
			}
			seen[cur] = struct{}{}
			f, ok := pcfg.Fixtures[cur]
			if !ok {
				return nil, fmt.Errorf("fixture %q not found", cur)
			}
			tree[f.Parent] = append(tree[f.Parent], cur)
			cur = f.Parent
		}
	}
	for _, children := range tree {
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

	// Traverse the tree to build a tree of fixtPlan.
	var traverse func(cur string) *fixtPlan
	traverse = func(cur string) *fixtPlan {
		var children []*fixtPlan
		for _, child := range tree[cur] {
			children = append(children, traverse(child))
		}
		return &fixtPlan{
			pcfg:     pcfg,
			fixt:     pcfg.Fixtures[cur],
			tests:    testMap[cur],
			children: children,
		}
	}
	return traverse(topFixt), nil
}

func (p *fixtPlan) run(ctx context.Context, st *fixtureStack, out OutputStream, dl *downloader) error {
	fi := p.fixt.EntityInfo()
	ce := &testing.CurrentEntity{
		ServiceDeps: fi.ServiceDeps,
		// TODO(crbug.com/1035940): Provide access to the output directory.
		// SoftwareDeps is not set; fixtures can't declare SoftwareDeps.
	}
	fout := newEntityOutputStream(out, fi)

	// Create a fixture-scoped context.
	logger := func(msg string) { fout.Log(msg) }
	ctx, cancel := context.WithCancel(testing.NewContext(ctx, ce, logger))
	defer cancel()

	// Set up the fixture if needed.
	if err := st.Push(ctx, p.fixt); err != nil {
		return err
	}
	// Do not defer st.Pop call here. It is correct to not call TearDown when
	// returning an error because it happens only when the timeout is ignored.

	// Run direct child tests first.
	for _, t := range p.tests {
		tout := newEntityOutputStream(out, t.EntityInfo())
		if st.Alive() {
			if err := runTest(ctx, t, tout, p.pcfg, st, &preConfig{}, dl); err != nil {
				return err
			}
			if err := st.Reset(ctx); err != nil {
				return err
			}
		} else {
			// TODO: Mark t as failed.
		}
	}

	// Run child fixtures.
	for _, c := range p.children {
		if err := c.run(ctx, st, out, dl); err != nil {
			return err
		}
	}

	// Tear down the fixture if needed.
	if err := st.Pop(ctx); err != nil {
		return err
	}
	return nil
}

func (p *fixtPlan) testsToRun() []*testing.TestInstance {
	tests := append([]*testing.TestInstance(nil), p.tests...)
	for _, c := range p.children {
		tests = append(tests, c.testsToRun()...)
	}
	return tests
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
		SoftwareDeps: append([]string(nil), p.tests[0].SoftwareDeps...),
		ServiceDeps:  append([]string(nil), p.tests[0].ServiceDeps...),
	}
	plog := newPreLogger(out)
	pctx, cancel := context.WithCancel(testing.NewContext(ctx, ec, plog.Log))
	defer cancel()

	// Create an empty fixture stack. Tests using preconditions can't depend on
	// fixtures.
	st := newFixtureStack(p.pcfg)

	for i, t := range p.tests {
		ti := t.EntityInfo()
		plog.SetCurrentTest(ti)
		tout := newEntityOutputStream(out, ti)
		precfg := &preConfig{
			ctx:   pctx,
			close: p.pre != nil && i == len(p.tests)-1,
		}
		if err := runTest(ctx, t, tout, p.pcfg, st, precfg, dl); err != nil {
			return err
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
func runTest(ctx context.Context, t *testing.TestInstance, tout *entityOutputStream, pcfg *Config, st *fixtureStack, precfg *preConfig, dl *downloader) error {
	dl.BeforeTest(ctx, t)

	// Attach a log that the test can use to report timing events.
	timingLog := timing.NewLog()
	ctx = timing.NewContext(ctx, timingLog)

	outDir, err := createEntityOutDir(pcfg.OutDir, t.Name)
	if err != nil {
		return err
	}

	tout.Start(outDir)
	defer tout.End(nil, timingLog)

	rcfg := &testing.RuntimeConfig{
		DataDir:      filepath.Join(pcfg.DataDir, testing.RelativeDataDir(t.Pkg)),
		OutDir:       outDir,
		Vars:         pcfg.Vars,
		CloudStorage: testing.NewCloudStorage(pcfg.Devservers),
		RemoteData:   pcfg.RemoteData,
		FixtureValue: st.Val(),
		PreCtx:       precfg.ctx,
		Purgeable:    dl.Purgeable(),
	}
	stages := buildStages(t, tout, pcfg, st, precfg, rcfg)

	ok := runStages(ctx, stages)
	if !ok {
		// If runStages reported that the test didn't finish, print diagnostic messages.
		const msg = "Test did not return on timeout (see log for goroutine dump)"
		tout.Error(testing.NewError(nil, msg, msg, 0))
		dumpGoroutines(tout)
	}

	if !ok {
		return errors.New("test did not return on timeout")
	}
	return nil
}

// downloader encapsulates the logic to download external data files.
type downloader struct {
	pcfg *Config
	cl   devserver.Client

	purgeable []string
}

func newDownloader(ctx context.Context, pcfg *Config) *downloader {
	cl := devserver.NewClient(ctx, pcfg.Devservers)
	return &downloader{
		pcfg: pcfg,
		cl:   cl,
	}
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

// buildStages builds stages to run a test.
//
// The time allotted to the test is generally the sum of t.Timeout and t.ExitTimeout, but
// additional time may be allotted for preconditions and pre/post-test hooks.
func buildStages(t *testing.TestInstance, tout testing.OutputStream, pcfg *Config, st *fixtureStack, precfg *preConfig, rcfg *testing.RuntimeConfig) []stage {
	var stages []stage
	addStage := func(f stageFunc, ctxTimeout, exitTimeout time.Duration) {
		stages = append(stages, stage{f, ctxTimeout, exitTimeout})
	}

	root := testing.NewTestEntityRoot(t, rcfg, tout)
	var postTestFunc func(ctx context.Context, s *testing.TestHookState)

	// First, perform setup and run the pre-test function.
	addStage(func(ctx context.Context) {
		root.RunWithTestState(ctx, func(ctx context.Context, s *testing.State) {
			// The test bundle is responsible for ensuring t.Timeout is nonzero before calling Run,
			// but we call s.Fatal instead of panicking since it's arguably nicer to report individual
			// test failures instead of aborting the entire run.
			if t.Timeout <= 0 {
				s.Fatal("Invalid timeout ", t.Timeout)
			}

			// Make sure all required data files exist.
			for _, fn := range t.Data {
				fp := s.DataPath(fn)
				if _, err := os.Stat(fp); err == nil {
					continue
				}
				ep := fp + testing.ExternalErrorSuffix
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
			if rcfg.RemoteData != nil {
				dt := s.DUT()
				if !dt.Connected(ctx) {
					s.Log("Reconnecting to DUT")
					if err := dt.Connect(ctx); err != nil {
						s.Fatal("Failed to reconnect to DUT: ", err)
					}
				}
			}
		})

		root.RunWithTestHookState(ctx, func(ctx context.Context, s *testing.TestHookState) {
			if pcfg.TestHook != nil {
				postTestFunc = pcfg.TestHook(ctx, s)
			}
		})
	}, preTestTimeout, exitTimeout)

	// TODO(crbug.com/1035940): Support fixture pre-test hooks.

	// Prepare the test's precondition (if any) if setup was successful.
	if t.Pre != nil {
		addStage(func(ctx context.Context) {
			if root.HasError() {
				return
			}
			root.RunWithPreState(ctx, func(ctx context.Context, s *testing.PreState) {
				s.Logf("Preparing precondition %q", t.Pre)
				root.SetPreValue(t.Pre.Prepare(ctx, s))
			})
		}, t.Pre.Timeout(), exitTimeout)
	}

	// Next, run the test function itself if no errors have been reported so far.
	addStage(func(ctx context.Context) {
		if root.HasError() {
			return
		}
		root.RunWithTestState(ctx, t.Func)
	}, t.Timeout, timeoutOrDefault(t.ExitTimeout, exitTimeout))

	// If this is the final test using this precondition, close it
	// (even if setup, t.Pre.Prepare, or t.Func failed).
	if precfg.close {
		addStage(func(ctx context.Context) {
			root.RunWithPreState(ctx, func(ctx context.Context, s *testing.PreState) {
				s.Logf("Closing precondition %q", t.Pre.String())
				t.Pre.Close(ctx, s)
			})
		}, t.Pre.Timeout(), exitTimeout)
	}

	// TODO(crbug.com/1035940): Support fixture post-test hooks.

	// Finally, run the post-test functions unconditionally.
	addStage(func(ctx context.Context) {
		root.RunWithTestHookState(ctx, func(ctx context.Context, s *testing.TestHookState) {
			if postTestFunc != nil {
				postTestFunc(ctx, s)
			}
		})
	}, postTestTimeout, exitTimeout)

	return stages
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
