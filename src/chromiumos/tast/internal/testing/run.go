// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"io/ioutil"
	"os"
	"time"
)

const (
	exitTimeout     = 3 * time.Second  // extra time granted to test-related funcs to exit
	preTestTimeout  = 15 * time.Second // timeout for TestConfig.PreTestFunc
	postTestTimeout = 15 * time.Second // timeout for TestConfig.PostTestFunc
)

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
func Run(ctx context.Context, t *TestInstance, ch chan<- Output, cfg *TestConfig) bool {
	// Attach the state to a context so support packages can log to it.
	root := NewRootState(t, ch, cfg)

	var stages []stage
	addStage := func(f stageFunc, ctxTimeout, runTimeout time.Duration) {
		stages = append(stages, stage{f, ctxTimeout, runTimeout})
	}

	var postTestHook func(ctx context.Context, s *State)

	// First, perform setup and run the pre-test function.
	addStage(func(ctx context.Context, root *RootState) {
		root.RunWithTestState(ctx, func(ctx context.Context, s *State) {
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
			if cfg.RemoteData != nil {
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
		})
	}, preTestTimeout, preTestTimeout+exitTimeout)

	// Prepare the test's precondition (if any) if setup was successful.
	if t.Pre != nil {
		addStage(func(ctx context.Context, root *RootState) {
			if root.HasError() {
				return
			}
			root.RunWithPreState(ctx, func(ctx context.Context, s *State) {
				s.Logf("Preparing precondition %q", t.Pre)

				if t.PreCtx == nil {
					// Associate PreCtx with TestContext for the first test.
					t.PreCtx, t.PreCtxCancel = context.WithCancel(NewContext(context.Background(), s))
				}

				if cfg.NextTest != nil && cfg.NextTest.Pre == t.Pre {
					cfg.NextTest.PreCtx = t.PreCtx
					cfg.NextTest.PreCtxCancel = t.PreCtxCancel
				}

				root.SetPreCtx(t.PreCtx)
				root.SetPreValue(t.Pre.(PreconditionImpl).Prepare(ctx, s))
			})
		}, t.Pre.Timeout(), t.Pre.Timeout()+exitTimeout)
	}

	// Next, run the test function itself if no errors have been reported so far.
	addStage(func(ctx context.Context, root *RootState) {
		if root.HasError() {
			return
		}
		root.RunWithTestState(ctx, t.Func)
	}, t.Timeout, t.Timeout+timeoutOrDefault(t.ExitTimeout, exitTimeout))

	// If this is the final test using this precondition, close it
	// (even if setup, t.Pre.Prepare, or t.Func failed).
	if t.Pre != nil && (cfg.NextTest == nil || cfg.NextTest.Pre != t.Pre) {
		addStage(func(ctx context.Context, root *RootState) {
			root.RunWithPreState(ctx, func(ctx context.Context, s *State) {
				s.Logf("Closing precondition %q", t.Pre.String())
				t.Pre.(PreconditionImpl).Close(ctx, s)
				if t.PreCtxCancel != nil {
					t.PreCtxCancel()
				}
			})
		}, t.Pre.Timeout(), t.Pre.Timeout()+exitTimeout)
	}

	// Finally, run the post-test functions unconditionally.
	addStage(func(ctx context.Context, root *RootState) {
		root.RunWithTestState(ctx, func(ctx context.Context, s *State) {
			if cfg.PostTestFunc != nil {
				cfg.PostTestFunc(ctx, s)
			}

			if postTestHook != nil {
				postTestHook(ctx, s)
			}
		})
	}, postTestTimeout, postTestTimeout+exitTimeout)

	return runStages(ctx, root, stages)
}

// timeoutOrDefault returns timeout if positive or def otherwise.
func timeoutOrDefault(timeout, def time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	return def
}
