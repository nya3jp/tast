// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/minidriver/bundleclient"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/testing"
	"go.chromium.org/tast/core/internal/xcontext"
)

// entityState is used by preprocessor to track the state of a single entity.
type entityState struct {
	Entity             *protocol.Entity
	Start              time.Time
	IntermediateOutDir string
	FinalOutDir        string

	Errors []*errorEntry
}

func (s *entityState) EntityInfo() *entityInfo {
	return &entityInfo{
		Entity:             s.Entity,
		Start:              s.Start,
		IntermediateOutDir: s.IntermediateOutDir,
		FinalOutDir:        s.FinalOutDir,
	}
}

// preprocessor processes test events before passing them to handlers.
// See the comments in processor.go for details.
type preprocessor struct {
	resDir   string
	diagnose DiagnoseFunc
	handlers []Handler

	stack      []*entityState
	copying    map[string]*entityState
	seenTimes  map[string]int
	fatalError *fatalError
	bundle     string
}

var _ bundleclient.RunTestsOutput = &preprocessor{}
var _ bundleclient.RunFixtureOutput = &preprocessor{}

func newPreprocessor(resDir string, diagnose DiagnoseFunc, handlers []Handler, bundle string) *preprocessor {
	return &preprocessor{
		resDir:   resDir,
		diagnose: diagnose,
		handlers: handlers,

		copying:   make(map[string]*entityState),
		seenTimes: make(map[string]int),
		bundle:    bundle,
	}
}

func (p *preprocessor) RunStart(ctx context.Context) error {
	var firstErr error
	for _, h := range p.handlers {
		if err := h.RunStart(ctx); err != nil && firstErr == nil {
			firstErr = errors.Wrap(err, "processing RunStart")
		}
	}
	return firstErr
}

func (p *preprocessor) EntityStart(ctx context.Context, ev *protocol.EntityStartEvent) error {
	outDir, err := p.createOutDir(ev.GetEntity())
	if err != nil {
		return errors.Wrapf(err, "processing EntityStart: failed to create an output directory for %s", ev.GetEntity().GetName())
	}

	err = ev.GetTime().CheckValid()
	if err != nil {
		return errors.Wrap(err, "processing EntityStart")
	}
	ts := ev.GetTime().AsTime()
	state := &entityState{
		Entity:             ev.GetEntity(),
		Start:              ts,
		IntermediateOutDir: ev.GetOutDir(),
		FinalOutDir:        outDir,
	}
	p.stack = append(p.stack, state)
	ei := state.EntityInfo()

	var firstErr error
	for _, h := range p.handlers {
		if err := h.EntityStart(ctx, ei); err != nil && firstErr == nil {
			firstErr = errors.Wrap(err, "processing EntityStart")
		}
	}
	return firstErr
}

func (p *preprocessor) EntityLog(ctx context.Context, ev *protocol.EntityLogEvent) error {
	state, err := p.stateOf(ev.GetEntityName())
	if err != nil {
		// Address b/389879153: We have seen something we will get EntityLog request from
		// previous test after connection to DUT is reestablished. Since this function
		// is only for logging we should not let it to fail the current test.
		// Therefore, we will not return a fatal error here.
		return nil
	}

	ts := ev.GetTime().AsTime()
	ei := state.EntityInfo()
	l := &logEntry{Time: ts, Text: ev.GetText(), Level: protocol.ProtoToLevel(ev.GetLevel())}

	var firstErr error
	for _, h := range p.handlers {
		if err := h.EntityLog(ctx, ei, l); err != nil && firstErr == nil {
			firstErr = errors.Wrap(err, "processing EntityLog")
		}
	}
	return firstErr
}

func (p *preprocessor) EntityError(ctx context.Context, ev *protocol.EntityErrorEvent) error {
	state, err := p.stateOf(ev.GetEntityName())
	if err != nil {
		return errors.Wrap(err, "processing EntityError")
	}

	err = ev.GetTime().CheckValid()
	if err != nil {
		return errors.Wrap(err, "processing EntityError")
	}
	ts := ev.GetTime().AsTime()
	e := &errorEntry{Time: ts, Error: ev.GetError()}
	state.Errors = append(state.Errors, e)

	ei := state.EntityInfo()

	var firstErr error
	for _, h := range p.handlers {
		if err := h.EntityError(ctx, ei, e); err != nil && firstErr == nil {
			firstErr = errors.Wrap(err, "processing EntityError")
		}
	}
	return firstErr
}

func (p *preprocessor) EntityEnd(ctx context.Context, ev *protocol.EntityEndEvent) error {
	state, err := p.stateOf(ev.GetEntityName())
	if err != nil {
		return errors.Wrap(err, "processing EntityEnd")
	}
	if stateTop := p.stateTop(); state != stateTop {
		return errors.Errorf("unexpected EntityEnd: got %q, want %q", state.Entity.GetName(), stateTop.Entity.GetName())
	}

	p.stack = p.stack[:len(p.stack)-1]
	p.copying[ev.GetEntityName()] = state

	err = ev.GetTime().CheckValid()
	if err != nil {
		return errors.Wrap(err, "processing EntityEnd")
	}
	ts := ev.GetTime().AsTime()
	ei := state.EntityInfo()
	result := &entityResult{
		Start:     state.Start,
		End:       ts,
		Skip:      ev.GetSkip(),
		Errors:    state.Errors,
		TimingLog: ev.GetTimingLog(),
	}

	var firstErr error
	for _, h := range p.handlers {
		if err := h.EntityEnd(ctx, ei, result); err != nil && firstErr == nil {
			firstErr = errors.Wrap(err, "processing EntityEnd")
		}
	}
	return firstErr
}

func (p *preprocessor) EntityCopyEnd(ctx context.Context, ev *protocol.EntityCopyEndEvent) error {
	state, ok := p.copying[ev.GetEntityName()]
	if !ok {
		return errors.Errorf("Unexpected EntityCopyEnd for entity %v", ev.GetEntityName())
	}
	delete(p.copying, ev.GetEntityName())
	ei := state.EntityInfo()

	var firstErr error
	for _, h := range p.handlers {
		if err := h.EntityCopyEnd(ctx, ei); err != nil && firstErr == nil {
			firstErr = errors.Wrap(err, "processing EntityCopyEnd")
		}
	}
	return firstErr
}

func (p *preprocessor) StackOperation(ctx context.Context, req *protocol.StackOperationRequest) *protocol.StackOperationResponse {
	var firstRes *protocol.StackOperationResponse
	for _, h := range p.handlers {
		res := h.StackOperation(ctx, req)
		if res == nil {
			continue
		}
		if firstRes != nil {
			return &protocol.StackOperationResponse{
				FatalError: "BUG: there should be only one hanlder that handles stack operation, but there are more than one",
			}
		}
		firstRes = res
	}
	return firstRes
}

func (p *preprocessor) RunLog(ctx context.Context, ev *protocol.RunLogEvent) error {
	ts := ev.GetTime().AsTime()
	l := &logEntry{Time: ts, Text: ev.GetText(), Level: protocol.ProtoToLevel(ev.GetLevel())}

	var firstErr error
	for _, h := range p.handlers {
		if err := h.RunLog(ctx, l); err != nil && firstErr == nil {
			firstErr = errors.Wrap(err, "processing RunLog")
		}
	}
	return firstErr
}

func (p *preprocessor) RunEnd(ctx context.Context, runErr error) {
	if runErr != nil {
		msg := fmt.Sprintf("Got global error: %+v", runErr)

		// Run diagnosis and replace the error message if it could give a more
		// detailed explanation.
		diagDir := p.resDir
		if len(p.stack) > 0 {
			diagDir = p.stateTop().FinalOutDir
		}
		if diagMsg := p.diagnose(ctx, diagDir); diagMsg != "" {
			msg = diagMsg
		}

		logging.Info(ctx, msg)

		// Attribute a run failure to the most recently started entity.
		if len(p.stack) > 0 {
			stateTop := p.stateTop()
			if ctxutil.DeadlineBefore(ctx, time.Now()) {
				apprRunTime := time.Since(stateTop.Start).Round(time.Millisecond).Seconds()
				if t, err := xcontext.GetContextTimeout(ctx); err == nil {
					msg = fmt.Sprintf("Test did not finish(~%v seconds) due to Tast command timeout(%v seconds)", apprRunTime, t.Seconds())
				}
			}
			// Always ignore errors from EntityError since runErr is non-nil.
			_ = p.EntityError(ctx, &protocol.EntityErrorEvent{
				Time:       timestamppb.Now(),
				EntityName: stateTop.Entity.GetName(),
				Error:      &protocol.Error{Reason: msg},
			})
		}
	}

	// Emit EntityError/EntityEnd events for orphan entities.
	// This loop will finish because an EntityEnd call pops an entityState
	// from the stack.
	for len(p.stack) > 0 {
		stateTop := p.stateTop()
		if err := p.EntityError(ctx, &protocol.EntityErrorEvent{
			Time:       timestamppb.Now(),
			EntityName: stateTop.Entity.GetName(),
			Error:      &protocol.Error{Reason: "Test did not finish"},
		}); err != nil && runErr == nil {
			runErr = err
		}
		if err := p.EntityEnd(ctx, &protocol.EntityEndEvent{
			Time:       timestamppb.Now(),
			EntityName: stateTop.Entity.GetName(),
		}); err != nil && runErr == nil {
			runErr = err
		}
	}

	// Finally, call RunEnd of handlers. runErr is already consumed
	// above, so we don't pass it to them.
	for _, h := range p.handlers {
		h.RunEnd(ctx)
	}

	// If runErr is *fatalError, save it.
	var fe *fatalError
	if errors.As(runErr, &fe) {
		p.fatalError = fe
	}
}

// FatalError returns a fatal error of the overall test execution if any.
//
// An error is considered fatal only if it should prevent the caller from
// retrying test execution any further. An example fatal error is that we've
// seen more test failures than allowed by the -maxtestfailures flag and should
// abort test execution immediately.
//
// Most errors are considered non-fatal and should be retried, e.g. test bundle
// crashes. Anyway, regardless of whether a test execution error is fatal or
// not, it's also reported as a test error so that it is visible in test
// results.
func (p *preprocessor) FatalError() error {
	if p.fatalError == nil {
		return nil
	}
	return p.fatalError
}

// stateTop returns entityState of the most recently started running entity.
func (p *preprocessor) stateTop() *entityState {
	return p.stack[len(p.stack)-1]
}

// stateOf returns entityState of a named running entity.
func (p *preprocessor) stateOf(name string) (*entityState, error) {
	for _, s := range p.stack {
		if s.Entity.GetName() == name {
			return s, nil
		}
	}
	return nil, errors.Errorf("no such entity running: %s", name)
}

// createOutDir creates an output directory for e, taking care of
// duplicated paths.
func (p *preprocessor) createOutDir(e *protocol.Entity) (string, error) {
	const (
		testLogsDir    = "tests"
		fixtureLogsDir = "fixtures"
		maxAttempts    = 200
	)

	dirName := testLogsDir
	if e.GetType() == protocol.EntityType_FIXTURE {
		dirName = fixtureLogsDir
	}
	defaultName := e.GetName()

	if e.GetName() == testing.TastRootRemoteFixtureName {
		// The remote root fixture is special because it will
		// be invoked once per bundle. It will be confusing which
		// run it is corresponding to.
		// Therefore, we add the bundle and timestamp suffix here
		// for easier debugging.
		timestamp := time.Now().UTC().Format("20060102150405")
		defaultName = fmt.Sprintf("%s_%s_%s", defaultName, p.bundle, timestamp)
	}
	relOutDir := filepath.Join(dirName, defaultName)

	// Add a number suffix to the output directory name in case of conflict.
	seenCnt := p.seenTimes[defaultName]
	if seenCnt > 0 {
		relOutDir += fmt.Sprintf(".%d", seenCnt)
	}
	p.seenTimes[e.GetName()]++

	outDir := filepath.Join(p.resDir, relOutDir)

	// Make sure the directory is unique.
	for i := 0; i < maxAttempts; i++ {
		if _, err := os.Stat(outDir); err != nil {
			break
		}
		relOutDir := filepath.Join(dirName,
			fmt.Sprintf("%s.%d", defaultName, seenCnt+i+1))
		outDir = filepath.Join(p.resDir, relOutDir)
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", err
	}
	return outDir, nil
}
