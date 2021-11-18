// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"

	"chromiumos/tast/internal/bundle/bundleclient"
	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/linuxssh"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/timing"
)

// testEntitiesToRun returns a sorted list of tests to run for the given names.
func testEntitiesToRun(allEntities []*protocol.ResolvedEntity, names []string) ([]*protocol.ResolvedEntity, error) {
	nameSet := make(map[string]struct{})
	for _, name := range names {
		nameSet[name] = struct{}{}
	}

	var tests []*protocol.ResolvedEntity
	for _, t := range allEntities {
		if t.Entity.Type != protocol.EntityType_TEST {
			continue
		}
		if _, ok := nameSet[t.GetEntity().GetName()]; ok {
			tests = append(tests, t)
		}
	}
	sort.Slice(tests, func(i, j int) bool {
		if tests[i].Hops != tests[j].Hops {
			return tests[i].Hops > tests[j].Hops
		}
		return tests[i].Entity.Name < tests[j].Entity.Name
	})
	return tests, nil
}

// runTestsRecursive runs tests per rcfg and scfg and writes responses to srv.
// If target bundle is specified in bundleParams, it runs tests on the target
// bundle too.
//
// If an error is encountered in the test harness (as opposed to in a test), an
// error is returned. Otherwise, nil is returned (test errors will be reported
// via EntityError control messages).
func runTestsRecursive(ctx context.Context, srv protocol.TestService_RunTestsServer, rcfg *protocol.RunConfig, scfg *StaticConfig, bundleParams *protocol.BundleInitParams) (retErr error) {
	ctx = testcontext.WithPrivateData(ctx, testcontext.PrivateData{
		WaitUntilReady: rcfg.GetWaitUntilReady(),
	})
	bcfg := bundleParams.GetBundleConfig()

	ew := newEventWriter(srv)

	hbw := newHeartbeatWriter(ew)
	defer hbw.Stop()

	ctx = logging.AttachLoggerNoPropagation(ctx, logging.NewSinkLogger(logging.LevelInfo, false, logging.NewFuncSink(func(msg string) {
		ew.RunLog(msg)
	})))

	var cl *bundleclient.Client
	if target := bcfg.GetPrimaryTarget(); target != nil {
		var err error
		cl, err = bundleclient.New(ctx, bcfg.GetPrimaryTarget(), scfg.registry.Name(), &protocol.HandshakeRequest{
			BundleInitParams: &protocol.BundleInitParams{
				Vars: bundleParams.Vars,
			},
		})
		if err != nil {
			return err
		}
		defer func() {
			if err := cl.Close(ctx); err != nil && retErr == nil {
				retErr = err
			}
		}()
	}
	es, err := listEntitiesRecursive(ctx, scfg.registry, rcfg.GetFeatures(), cl)
	if err != nil {
		return err
	}
	testEntities, err := testEntitiesToRun(es, rcfg.GetTests())
	if err != nil {
		return err
	}

	connEnv, err := setUpConnection(ctx, scfg, rcfg, bcfg)
	if err != nil {
		return err
	}
	defer connEnv.close(ctx)

	// Set up environment and create pcfg early so that we can run remote
	// fixtures for local tests.
	env, err := setUpTestEnvironment(ctx, scfg, rcfg, bcfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := env.close(ctx); err != nil && retErr != nil {
			retErr = err
		}
	}()

	pcfg := &planner.Config{
		Dirs:             rcfg.GetDirs(),
		Features:         rcfg.GetFeatures(),
		Service:          rcfg.GetServiceConfig(),
		DataFile:         rcfg.GetDataFileConfig(),
		RemoteData:       connEnv.rd,
		TestHook:         scfg.testHook,
		BeforeDownload:   scfg.beforeDownload,
		Fixtures:         scfg.registry.AllFixtures(),
		StartFixtureName: rcfg.GetStartFixtureState().GetName(),
		StartFixtureImpl: &stubFixture{setUpErrors: rcfg.GetStartFixtureState().GetErrors()},
	}

	// Run all the tests with Hops > 0 (i.e. local tests).
	testNames := make(map[string][]string)
	for _, e := range testEntities {
		if e.Hops == 0 {
			continue
		}
		testNames[e.StartFixtureName] = append(testNames[e.StartFixtureName], e.Entity.Name)
	}
	var startFixtures []string
	for f := range testNames {
		startFixtures = append(startFixtures, f)
	}
	sort.Strings(startFixtures)
	for _, f := range startFixtures {
		if err := runRemoteFixtureAndLocalTests(ctx, f, testNames[f], ew, pcfg, rcfg, bcfg, cl); err != nil {
			return err
		}
	}

	// Run all the tests with Hops = 0.
	nameToTest := make(map[string]*testing.TestInstance)
	for _, t := range scfg.registry.AllTests() {
		nameToTest[t.Name] = t
	}
	var tests []*testing.TestInstance
	for _, t := range testEntities {
		if t.Hops > 0 {
			continue
		}
		tests = append(tests, nameToTest[t.Entity.Name])
	}
	if err := planner.RunTests(ctx, tests, ew, pcfg); err != nil {
		return command.NewStatusErrorf(statusError, "run failed: %v", err)
	}
	return nil
}

func runRemoteFixtureAndLocalTests(
	ctx context.Context,
	startFixture string,
	tests []string,
	ew *eventWriter,
	pcfg *planner.Config,
	rcfg *protocol.RunConfig,
	bcfg *protocol.BundleConfig,
	cl *bundleclient.Client,
) (retErr error) {
	var setUpErrors []*protocol.Error
	if startFixture != "" {
		f, ok := pcfg.Fixtures[startFixture]
		if !ok {
			setUpErrors = append(setUpErrors, &protocol.Error{
				Reason: fmt.Sprintf("fixture %q not found", startFixture),
			})
		} else {
			out := &collectErrorsOutputStream{out: ew}
			stack := planner.NewFixtureStack(pcfg, out)
			if err := stack.Push(ctx, f); err != nil {
				return fmt.Errorf("push remote fixture %q: %v", f.Name, err)
			}
			setUpErrors = out.errs
			defer func() {
				if err := stack.Pop(ctx); err != nil && retErr == nil {
					retErr = err
				}
			}()
		}
	}
	return runLocalTests(ctx, startFixture, setUpErrors, tests, ew, rcfg, bcfg, cl)
}

func runLocalTests(
	ctx context.Context,
	startFixture string,
	setUpErrors []*protocol.Error,
	tests []string,
	ew *eventWriter,
	rcfg *protocol.RunConfig,
	bcfg *protocol.BundleConfig,
	cl *bundleclient.Client) (retErr error) {

	rcl, err := cl.TestService().RunTests(ctx)
	if err != nil {
		return err
	}

	if err := rcl.Send(&protocol.RunTestsRequest{
		Type: &protocol.RunTestsRequest_RunTestsInit{
			RunTestsInit: &protocol.RunTestsInit{
				RunConfig: &protocol.RunConfig{
					Tests:    tests,
					Dirs:     rcfg.GetDirs().GetPrimaryTarget(),
					Features: rcfg.GetFeatures(),
					ServiceConfig: &protocol.ServiceConfig{
						Devservers:  rcfg.GetServiceConfig().GetDevservers(),
						TlwServer:   rcfg.GetServiceConfig().GetTlwServer(),
						TlwSelfName: "", // TODO: fill it
					},
					DataFileConfig: rcfg.GetDataFileConfig(),
					StartFixtureState: &protocol.StartFixtureState{
						Name:   startFixture,
						Errors: setUpErrors,
					},
					HeartbeatInterval: rcfg.GetHeartbeatInterval(),
					WaitUntilReady:    rcfg.GetWaitUntilReady(),
				},
				Recursive: true,
			},
		},
	}); err != nil {
		return err
	}

	var outDirStack []string // empty string is pushed if entity is skipped
	for {
		resp, err := rcl.Recv()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		switch x := resp.Type.(type) {
		case *protocol.RunTestsResponse_EntityStart:
			outDirStack = append(outDirStack, x.EntityStart.OutDir)
		case *protocol.RunTestsResponse_EntityEnd:
			src := outDirStack[len(outDirStack)-1]
			outDirStack = outDirStack[:len(outDirStack)-1]
			if src == "" {
				break
			}
			name := x.EntityEnd.EntityName
			dst := filepath.Join(rcfg.GetDirs().GetOutDir(), name)
			if err := linuxssh.GetAndDeleteFile(ctx, cl.SSHConn(), src, dst, linuxssh.PreserveSymlinks); err != nil {
				return err
			}
		}
		if err := ew.srv.Send(resp); err != nil {
			return err
		}
	}

	// TODO: Diagnose SSH connection drops.
	return nil
}

type collectErrorsOutputStream struct {
	out  planner.OutputStream
	errs []*protocol.Error
}

func (s *collectErrorsOutputStream) EntityStart(ei *protocol.Entity, outDir string) error {
	return s.out.EntityStart(ei, outDir)
}

func (s *collectErrorsOutputStream) EntityLog(ei *protocol.Entity, msg string) error {
	return s.out.EntityLog(ei, msg)
}

func (s *collectErrorsOutputStream) EntityError(ei *protocol.Entity, e *protocol.Error) error {
	s.errs = append(s.errs, e)
	return s.out.EntityError(ei, e)
}

func (s *collectErrorsOutputStream) EntityEnd(ei *protocol.Entity, skipReasons []string, timingLog *timing.Log) error {
	return s.out.EntityEnd(ei, skipReasons, timingLog)
}
