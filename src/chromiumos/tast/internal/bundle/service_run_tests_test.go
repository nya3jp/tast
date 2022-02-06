// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle_test

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"sync"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/bundle/bundletest"
	"chromiumos/tast/internal/fakesshserver"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/protocol/protocoltest"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/testing/testfixture"
	"chromiumos/tast/testutil"
)

func setUpWithDefaultDUT(t *gotesting.T, os ...bundletest.Option) *bundletest.Env {
	defaultPrimaryDUT := bundletest.WithPrimaryDUT(&bundletest.DUTConfig{
		ExtraSSHHandlers: []fakesshserver.Handler{
			fakesshserver.ExactMatchHandler("exec cat /proc/sys/kernel/random/boot_id", func(_ io.Reader, stdout, stderr io.Writer) int {
				io.WriteString(stdout, "fake-boot-id")
				return 0
			}),
		},
	})

	opts := append(append([]bundletest.Option(nil), os...), defaultPrimaryDUT)
	return bundletest.SetUp(
		t,
		opts...,
	)
}

func TestTestServiceRunTests(t *gotesting.T) {
	localTest1 := &testing.TestInstance{
		Name: "foo.Local1",
		Pkg:  "foo",
		Data: []string{"data.txt"},
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Local test 1")
			ioutil.WriteFile(filepath.Join(s.OutDir(), "out.txt"), []byte("local1"), 0644)
			b, err := ioutil.ReadFile(s.DataPath("data.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if got, want := string(b), "local data"; got != want {
				t.Errorf("Got %v, want %v", got, want)
			}
		},
		Fixture: "remoteFixture",
		Timeout: time.Minute,
	}
	localTest2 := &testing.TestInstance{
		Name: "foo.Local2",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Local test 2")
			ioutil.WriteFile(filepath.Join(s.OutDir(), "out.txt"), []byte("local2"), 0644)
		},
		Fixture: "remoteFixture",
		Timeout: time.Minute,
	}

	remoteTest := &testing.TestInstance{
		Name: "foo.Remote",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Remote testing")
			ioutil.WriteFile(filepath.Join(s.OutDir(), "out.txt"), []byte("remote"), 0644)
		},
		Timeout: time.Minute,
	}
	parentRemoteFixture := &testing.FixtureInstance{
		Name: "parentRemoteFixture",
		Pkg:  "fixt",
		Impl: testfixture.New(
			testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				s.Log("Parent fixture SetUp")
				return nil
			}),
		),
	}
	remoteFixture := &testing.FixtureInstance{
		Name:   "remoteFixture",
		Parent: "parentRemoteFixture",
		Pkg:    "fixt",
		Data:   []string{"data.txt"},
		Impl: testfixture.New(
			testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				s.Log("Remote fixture SetUp")
				b, err := ioutil.ReadFile(s.DataPath("data.txt"))
				if err != nil {
					t.Fatal(err)
				}
				if got, want := string(b), "fixture data"; got != want {
					t.Errorf("Got %v, want %v", got, want)
				}
				if err := ioutil.WriteFile(filepath.Join(s.OutDir(), "set_up.txt"), []byte("set up"), 0644); err != nil {
					t.Error(err)
				}
				return nil
			}),
			testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
				s.Log("Remote fixture TearDown")
				if err := ioutil.WriteFile(filepath.Join(s.OutDir(), "tear_down.txt"), []byte("tear down"), 0644); err != nil {
					t.Error(err)
				}
			}),
			testfixture.WithPreTest(func(ctx context.Context, s *testing.FixtTestState) {
				s.Log("Remote fixture PreTest")
				if err := ioutil.WriteFile(filepath.Join(s.OutDir(), "pre_test.txt"), []byte("pre test"), 0644); err != nil {
					t.Error(err)
				}
			}),
			testfixture.WithPostTest(func(ctx context.Context, s *testing.FixtTestState) {
				s.Log("Remote fixture PostTest")
				if err := ioutil.WriteFile(filepath.Join(s.OutDir(), "post_test.txt"), []byte("post test"), 0644); err != nil {
					t.Error(err)
				}
			}),
			testfixture.WithReset(func(ctx context.Context) error {
				logging.Info(ctx, "Remote fixture Reset")

				dir, ok := testcontext.OutDir(ctx)
				if !ok {
					t.Error("No OutDir access in Reset")
				}
				if err := ioutil.WriteFile(filepath.Join(dir, "reset.txt"), []byte("reset"), 0644); err != nil {
					t.Error(err)
				}
				return nil
			}),
		),
	}

	localReg := testing.NewRegistry("bundle")
	localReg.AddTestInstance(localTest1)
	localReg.AddTestInstance(localTest2)

	remoteReg := testing.NewRegistry("bundle")
	remoteReg.AddTestInstance(remoteTest)
	remoteReg.AddFixtureInstance(remoteFixture)
	remoteReg.AddFixtureInstance(parentRemoteFixture)

	env := setUpWithDefaultDUT(
		t,
		bundletest.WithLocalBundles(localReg),
		bundletest.WithRemoteBundles(remoteReg),
		bundletest.WithLocalData(map[string]string{
			"foo/data/data.txt": "local data",
		}),
		bundletest.WithRemoteData(map[string]string{
			"fixt/data/data.txt": "fixture data",
		}),
	)
	cl := protocol.NewTestServiceClient(env.DialRemoteBundle(context.Background(), t, remoteReg.Name()))

	events, err := protocoltest.RunTestsRecursiveForEvents(context.Background(),
		cl, env.RunConfig(), protocoltest.WithEntityLogs(),
	)
	if err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}

	wantEvents := []protocol.Event{
		// Fixture set up
		&protocol.EntityStartEvent{Entity: parentRemoteFixture.EntityProto()},
		&protocol.EntityLogEvent{EntityName: parentRemoteFixture.Name, Text: "Parent fixture SetUp"},
		&protocol.EntityStartEvent{Entity: remoteFixture.EntityProto()},
		&protocol.EntityLogEvent{EntityName: remoteFixture.Name, Text: "Remote fixture SetUp"},
		// First test
		&protocol.EntityStartEvent{Entity: localTest1.EntityProto()},
		&protocol.EntityLogEvent{EntityName: localTest1.Name, Text: "Remote fixture PreTest"},
		&protocol.EntityLogEvent{EntityName: localTest1.Name, Text: "Local test 1"},
		&protocol.EntityLogEvent{EntityName: localTest1.Name, Text: "Remote fixture PostTest"},
		&protocol.EntityEndEvent{EntityName: localTest1.Name},
		&protocol.EntityLogEvent{EntityName: remoteFixture.Name, Text: "Remote fixture Reset"},
		// Second test
		&protocol.EntityStartEvent{Entity: localTest2.EntityProto()},
		&protocol.EntityLogEvent{EntityName: localTest2.Name, Text: "Remote fixture PreTest"},
		&protocol.EntityLogEvent{EntityName: localTest2.Name, Text: "Local test 2"},
		&protocol.EntityLogEvent{EntityName: localTest2.Name, Text: "Remote fixture PostTest"},
		&protocol.EntityEndEvent{EntityName: localTest2.Name},
		// Fixture tear down
		&protocol.EntityLogEvent{EntityName: remoteFixture.Name, Text: "Remote fixture TearDown"},
		&protocol.EntityEndEvent{EntityName: remoteFixture.Name},
		&protocol.EntityEndEvent{EntityName: parentRemoteFixture.Name},
		// Remote test
		&protocol.EntityStartEvent{Entity: remoteTest.EntityProto()},
		&protocol.EntityLogEvent{EntityName: remoteTest.Name, Text: "Remote testing"},
		&protocol.EntityEndEvent{EntityName: remoteTest.Name},
	}

	if diff := cmp.Diff(events, wantEvents, protocoltest.EventCmpOpts...); diff != "" {
		t.Errorf("Events mismatch (-got +want):\n%s", diff)
	}

	m, err := testutil.ReadFiles(env.RemoteOutDir())
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(m, map[string]string{
		"tests/foo.Local1/out.txt":    "local1",
		"tests/foo.Local2/out.txt":    "local2",
		"foo.Local1/pre_test.txt":     "pre test",
		"foo.Local1/post_test.txt":    "post test",
		"foo.Local2/pre_test.txt":     "pre test",
		"foo.Local2/post_test.txt":    "post test",
		"foo.Remote/out.txt":          "remote",
		"remoteFixture/set_up.txt":    "set up",
		"remoteFixture/tear_down.txt": "tear down",
		"remoteFixture/reset.txt":     "reset",
	}); diff != "" {
		t.Errorf("OutDir mismatch (-got +want):\n%s", diff)
	}

	m, err = testutil.ReadFiles(env.LocalOutDir())
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(m, map[string]string{}); diff != "" {
		t.Errorf("Local OutDir is non-empty unexpectedly (-got +want):\n%s", diff)
	}
}

func TestTestServiceRunTests_SimpleRemoteLocal(t *gotesting.T) {
	localTest := &testing.TestInstance{
		Name:    "foo.Local",
		Func:    func(ctx context.Context, s *testing.State) {},
		Fixture: "remoteFixture",
		Timeout: time.Minute,
	}
	remoteFixture := &testing.FixtureInstance{
		Name: "remoteFixture",
		Pkg:  "fixt",
		Impl: testfixture.New(),
	}

	localReg := testing.NewRegistry("bundle")
	localReg.AddTestInstance(localTest)

	remoteReg := testing.NewRegistry("bundle")
	remoteReg.AddFixtureInstance(remoteFixture)

	env := setUpWithDefaultDUT(
		t,
		bundletest.WithLocalBundles(localReg),
		bundletest.WithRemoteBundles(remoteReg),
	)
	cl := protocol.NewTestServiceClient(env.DialRemoteBundle(context.Background(), t, remoteReg.Name()))

	events, err := protocoltest.RunTestsRecursiveForEvents(context.Background(),
		cl, env.RunConfig(),
	)
	if err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}

	wantEvents := []protocol.Event{
		&protocol.EntityStartEvent{Entity: remoteFixture.EntityProto()},
		&protocol.EntityStartEvent{Entity: localTest.EntityProto()},
		&protocol.EntityEndEvent{EntityName: localTest.Name},
		&protocol.EntityEndEvent{EntityName: remoteFixture.Name},
	}

	if diff := cmp.Diff(events, wantEvents, protocoltest.EventCmpOpts...); diff != "" {
		t.Errorf("Events mismatch (-got +want):\n%s", diff)
	}
}

func TestTestServiceRunTests_NoTests(t *gotesting.T) {
	env := setUpWithDefaultDUT(t,
		bundletest.WithRemoteBundles(testing.NewRegistry("bundle")),
		bundletest.WithLocalBundles(testing.NewRegistry("bundle")),
	)
	cl := protocol.NewTestServiceClient(env.DialRemoteBundle(context.Background(), t, "bundle"))
	if _, err := protocoltest.RunTestsRecursiveForEvents(context.Background(), cl, env.RunConfig()); err != nil {
		t.Fatalf("RunTests failed for empty tests: %v", err)
	}
}

func TestTestServiceRunTests_RemoteOnly(t *gotesting.T) {
	remoteTest := &testing.TestInstance{
		Name:    "foo.Remote",
		Fixture: "remoteFixture",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Remote testing")
		},
		Timeout: time.Minute,
	}
	remoteFixture := &testing.FixtureInstance{
		Name: "remoteFixture",
		Pkg:  "fixt",
		Impl: testfixture.New(
			testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				s.Log("Remote fixture SetUp")
				return nil
			}),
			testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
				s.Log("Remote fixture TearDown")
			}),
		),
	}

	remoteReg := testing.NewRegistry("bundle")
	remoteReg.AddTestInstance(remoteTest)
	remoteReg.AddFixtureInstance(remoteFixture)

	env := setUpWithDefaultDUT(
		t,
		bundletest.WithLocalBundles(testing.NewRegistry("bundle")),
		bundletest.WithRemoteBundles(remoteReg),
	)
	cl := protocol.NewTestServiceClient(env.DialRemoteBundle(context.Background(), t, remoteReg.Name()))

	events, err := protocoltest.RunTestsRecursiveForEvents(context.Background(),
		cl, env.RunConfig(), protocoltest.WithEntityLogs(),
	)
	if err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}

	wantEvents := []protocol.Event{
		&protocol.EntityStartEvent{Entity: remoteFixture.EntityProto()},
		&protocol.EntityLogEvent{EntityName: remoteFixture.Name, Text: "Remote fixture SetUp"},
		&protocol.EntityStartEvent{Entity: remoteTest.EntityProto()},
		&protocol.EntityLogEvent{EntityName: remoteTest.Name, Text: "Remote testing"},
		&protocol.EntityEndEvent{EntityName: remoteTest.Name},
		&protocol.EntityLogEvent{EntityName: remoteFixture.Name, Text: "Remote fixture TearDown"},
		&protocol.EntityEndEvent{EntityName: remoteFixture.Name},
	}

	if diff := cmp.Diff(events, wantEvents, protocoltest.EventCmpOpts...); diff != "" {
		t.Errorf("Events mismatch (-got +want):\n%s", diff)
	}
}

func TestTestServiceRunTests_LocalOnly(t *gotesting.T) {
	localTest := &testing.TestInstance{
		Name:    "foo.Local",
		Fixture: "localFixture",
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Local testing")
		},
		Timeout: time.Minute,
	}
	localFixture := &testing.FixtureInstance{
		Name: "localFixture",
		Pkg:  "fixt",
		Impl: testfixture.New(
			testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				s.Log("Local fixture SetUp")
				return nil
			}),
			testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
				s.Log("Local fixture TearDown")
			}),
		),
	}

	localReg := testing.NewRegistry("bundle")
	localReg.AddTestInstance(localTest)
	localReg.AddFixtureInstance(localFixture)

	env := setUpWithDefaultDUT(
		t,
		bundletest.WithLocalBundles(localReg),
		bundletest.WithRemoteBundles(testing.NewRegistry("bundle")),
	)
	cl := protocol.NewTestServiceClient(env.DialRemoteBundle(context.Background(), t, localReg.Name()))

	events, err := protocoltest.RunTestsRecursiveForEvents(context.Background(),
		cl, env.RunConfig(), protocoltest.WithEntityLogs(),
	)
	if err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}

	wantEvents := []protocol.Event{
		&protocol.EntityStartEvent{Entity: localFixture.EntityProto()},
		&protocol.EntityLogEvent{EntityName: localFixture.Name, Text: "Local fixture SetUp"},
		&protocol.EntityStartEvent{Entity: localTest.EntityProto()},
		&protocol.EntityLogEvent{EntityName: localTest.Name, Text: "Local testing"},
		&protocol.EntityEndEvent{EntityName: localTest.Name},
		&protocol.EntityLogEvent{EntityName: localFixture.Name, Text: "Local fixture TearDown"},
		&protocol.EntityEndEvent{EntityName: localFixture.Name},
	}
	if diff := cmp.Diff(events, wantEvents, protocoltest.EventCmpOpts...); diff != "" {
		t.Errorf("Events mismatch (-got +want):\n%s", diff)
	}
}

func TestTestServiceRunTests_Failures(t *gotesting.T) {
	localPass := &testing.TestInstance{
		Name:    "local.Pass",
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
	}
	localFail := &testing.TestInstance{
		Name:    "local.Fail",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Error("Failed")
		},
	}
	localSkip := &testing.TestInstance{
		Name:         "local.Skip",
		Timeout:      time.Minute,
		SoftwareDeps: []string{"missing"},
		Func:         func(ctx context.Context, s *testing.State) {},
	}
	localReg := testing.NewRegistry("bundle")
	localReg.AddTestInstance(localPass)
	localReg.AddTestInstance(localFail)
	localReg.AddTestInstance(localSkip)

	remotePass := &testing.TestInstance{
		Name:    "remote.Pass",
		Timeout: time.Minute,
		Func:    func(ctx context.Context, s *testing.State) {},
	}
	remoteFail := &testing.TestInstance{
		Name:    "remote.Fail",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Error("Failed")
		},
	}
	remoteSkip := &testing.TestInstance{
		Name:         "remote.Skip",
		Timeout:      time.Minute,
		SoftwareDeps: []string{"missing"},
		Func:         func(ctx context.Context, s *testing.State) {},
	}
	remoteReg := testing.NewRegistry("bundle")
	remoteReg.AddTestInstance(remotePass)
	remoteReg.AddTestInstance(remoteFail)
	remoteReg.AddTestInstance(remoteSkip)

	env := setUpWithDefaultDUT(t,
		bundletest.WithLocalBundles(localReg),
		bundletest.WithRemoteBundles(remoteReg),
	)
	cl := protocol.NewTestServiceClient(env.DialRemoteBundle(context.Background(), t, "bundle"))

	events, err := protocoltest.RunTestsRecursiveForEvents(context.Background(), cl, env.RunConfig())
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := []protocol.Event{
		// Report skipped tests first.
		&protocol.EntityStartEvent{Entity: localSkip.EntityProto()},
		&protocol.EntityErrorEvent{EntityName: localSkip.Name, Error: &protocol.Error{Reason: "unknown SoftwareDeps: missing"}},
		&protocol.EntityEndEvent{EntityName: localSkip.Name},
		&protocol.EntityStartEvent{Entity: localFail.EntityProto()},
		&protocol.EntityErrorEvent{EntityName: localFail.Name, Error: &protocol.Error{Reason: "Failed"}},
		&protocol.EntityEndEvent{EntityName: localFail.Name},
		&protocol.EntityStartEvent{Entity: localPass.EntityProto()},
		&protocol.EntityEndEvent{EntityName: localPass.Name},
		&protocol.EntityStartEvent{Entity: remoteSkip.EntityProto()},
		&protocol.EntityErrorEvent{EntityName: remoteSkip.Name, Error: &protocol.Error{Reason: "unknown SoftwareDeps: missing"}},
		&protocol.EntityEndEvent{EntityName: remoteSkip.Name},
		&protocol.EntityStartEvent{Entity: remoteFail.EntityProto()},
		&protocol.EntityErrorEvent{EntityName: remoteFail.Name, Error: &protocol.Error{Reason: "Failed"}},
		&protocol.EntityEndEvent{EntityName: remoteFail.Name},
		&protocol.EntityStartEvent{Entity: remotePass.EntityProto()},
		&protocol.EntityEndEvent{EntityName: remotePass.Name},
	}

	if diff := cmp.Diff(events, wantEvents, protocoltest.EventCmpOpts...); diff != "" {
		t.Errorf("Events mismatch (-got +want):\n%s", diff)
	}
}

func TestTestServiceRunTests_LocalTestRemoteFixtureFailures(t *gotesting.T) {
	localNoSuchFixture := &testing.TestInstance{
		Name:    "local.NoSuchFixture",
		Fixture: "noSuchFixture",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Unexpected log")
		},
	}
	localFailSetUpRemoteFixture := &testing.TestInstance{
		Name:    "local.FailSetUpRemoteFixture",
		Fixture: "failSetUpRemoteFixture",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Unexpected log")
		},
	}
	localFailTearDownRemoteFixture := &testing.TestInstance{
		Name:    "local.FailTearDownRemoteFixture",
		Fixture: "failTearDownRemoteFixture",
		Timeout: time.Minute,
		Func: func(ctx context.Context, s *testing.State) {
			s.Log("Test run")
		},
	}
	localReg := testing.NewRegistry("bundle")
	localReg.AddTestInstance(localNoSuchFixture)
	localReg.AddTestInstance(localFailSetUpRemoteFixture)
	localReg.AddTestInstance(localFailTearDownRemoteFixture)

	failSetUpRemoteFixture := &testing.FixtureInstance{
		Name: "failSetUpRemoteFixture",
		Impl: testfixture.New(testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
			s.Error("SetUp fail")
			return nil
		})),
	}
	failTearDownRemoteFixture := &testing.FixtureInstance{
		Name: "failTearDownRemoteFixture",
		Impl: testfixture.New(testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
			s.Error("TearDown fail")
		})),
	}
	remoteReg := testing.NewRegistry("bundle")
	remoteReg.AddFixtureInstance(failSetUpRemoteFixture)
	remoteReg.AddFixtureInstance(failTearDownRemoteFixture)

	env := setUpWithDefaultDUT(t,
		bundletest.WithLocalBundles(localReg),
		bundletest.WithRemoteBundles(remoteReg),
	)
	cl := protocol.NewTestServiceClient(env.DialRemoteBundle(context.Background(), t, remoteReg.Name()))

	events, err := protocoltest.RunTestsRecursiveForEvents(context.Background(), cl, env.RunConfig(), protocoltest.WithEntityLogs())
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := []protocol.Event{
		// Orphan tests are reported first.
		&protocol.EntityStartEvent{Entity: localNoSuchFixture.EntityProto()},
		&protocol.EntityErrorEvent{EntityName: localNoSuchFixture.Name, Error: &protocol.Error{Reason: `Fixture "noSuchFixture" not found`}},
		&protocol.EntityEndEvent{EntityName: localNoSuchFixture.Name},

		&protocol.EntityStartEvent{Entity: failSetUpRemoteFixture.EntityProto()},
		&protocol.EntityErrorEvent{EntityName: failSetUpRemoteFixture.Name, Error: &protocol.Error{Reason: "SetUp fail"}},
		&protocol.EntityEndEvent{EntityName: failSetUpRemoteFixture.Name},
		&protocol.EntityStartEvent{Entity: localFailSetUpRemoteFixture.EntityProto()},
		&protocol.EntityErrorEvent{EntityName: localFailSetUpRemoteFixture.Name, Error: &protocol.Error{Reason: "[Fixture failure] failSetUpRemoteFixture: SetUp fail"}},
		&protocol.EntityEndEvent{EntityName: localFailSetUpRemoteFixture.Name},

		&protocol.EntityStartEvent{Entity: failTearDownRemoteFixture.EntityProto()},
		&protocol.EntityStartEvent{Entity: localFailTearDownRemoteFixture.EntityProto()},
		&protocol.EntityLogEvent{EntityName: localFailTearDownRemoteFixture.Name, Text: "Test run"},
		&protocol.EntityEndEvent{EntityName: localFailTearDownRemoteFixture.Name},
		&protocol.EntityErrorEvent{EntityName: failTearDownRemoteFixture.Name, Error: &protocol.Error{Reason: "TearDown fail"}},
		&protocol.EntityEndEvent{EntityName: failTearDownRemoteFixture.Name},
	}
	if diff := cmp.Diff(events, wantEvents, protocoltest.EventCmpOpts...); diff != "" {
		t.Errorf("Events mismatch (-got +want):\n%s", diff)
	}
}

func TestTestServiceRunTests_ExternalPreTest(t *gotesting.T) {
	localTest := &testing.TestInstance{
		Name:    "local.Foo",
		Fixture: "fixture",
		Timeout: time.Hour,
		Func: func(ctx context.Context, s *testing.State) {
			s.Error("Test error")
		},
	}
	localReg := testing.NewRegistry("bundle")
	localReg.AddTestInstance(localTest)

	var wg sync.WaitGroup
	wg.Add(2)

	var env *bundletest.Env
	fixture := &testing.FixtureInstance{
		Name:            "fixture",
		PreTestTimeout:  time.Hour,
		PostTestTimeout: time.Hour,
		TearDownTimeout: time.Hour,
		Impl: testfixture.New(
			testfixture.WithPreTest(func(ctx context.Context, s *testing.FixtTestState) {
				if s.HasError() {
					t.Error("t.HasError() = true, want false")
				}
				if got, want := s.DUT().HostName(), env.PrimaryServer(); got != want {
					t.Errorf("DUT host name = %v, want %v", got, want)
				}
				if s.CloudStorage() == nil {
					t.Error("s.CloudStorage() = nil, want non nil")
				}
				if s.RPCHint() == nil {
					t.Error("s.RPCHint() = nil, want non nil")
				}

				if err := s.TestContext().Err(); err != nil {
					t.Errorf("s.TestContext() is canceled: %v", err)
				}
				go func() {
					<-s.TestContext().Done()
					wg.Done()
				}()
			}),
			testfixture.WithPostTest(func(ctx context.Context, s *testing.FixtTestState) {
				if !s.HasError() {
					t.Error("t.HasError() = false, want true")
				}
				if got, want := s.DUT().HostName(), env.PrimaryServer(); got != want {
					t.Errorf("DUT host name = %v, want %v", got, want)
				}
				if s.CloudStorage() == nil {
					t.Error("s.CloudStorage() = nil, want non nil")
				}
				if s.RPCHint() == nil {
					t.Error("s.RPCHint() = nil, want non nil")
				}

				if err := s.TestContext().Err(); err != nil {
					t.Errorf("s.TestContext() is canceled: %v", err)
				}
				go func() {
					<-s.TestContext().Done()
					wg.Done()
				}()
			}),
			testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
				// TestContext must be cancelled as soon as PostTest finishes.
				wg.Wait()
			}),
		),
	}
	remoteReg := testing.NewRegistry("bundle")
	remoteReg.AddFixtureInstance(fixture)

	env = setUpWithDefaultDUT(t, bundletest.WithLocalBundles(localReg), bundletest.WithRemoteBundles(remoteReg))
	cl := protocol.NewTestServiceClient(env.DialRemoteBundle(context.Background(), t, remoteReg.Name()))

	events, err := protocoltest.RunTestsRecursiveForEvents(context.Background(), cl, env.RunConfig(), protocoltest.WithEntityLogs())
	if err != nil {
		t.Fatal(err)
	}
	wg.Wait()

	wantEvents := []protocol.Event{
		&protocol.EntityStartEvent{Entity: fixture.EntityProto()},
		&protocol.EntityStartEvent{Entity: localTest.EntityProto()},
		&protocol.EntityErrorEvent{EntityName: localTest.Name, Error: &protocol.Error{Reason: "Test error"}},
		&protocol.EntityEndEvent{EntityName: localTest.Name},
		&protocol.EntityEndEvent{EntityName: fixture.Name},
	}

	if diff := cmp.Diff(events, wantEvents, protocoltest.EventCmpOpts...); diff != "" {
		t.Errorf("Events mismatch (-got +want):\n%s", diff)
	}
}

func TestTestServiceRunTests_ExternalResetFailure(t *gotesting.T) {
	ctx := context.Background()

	localTest := &testing.TestInstance{
		Name:    "local.Foo",
		Fixture: "fixture",
		Timeout: time.Hour,
		Func:    func(ctx context.Context, s *testing.State) {},
	}
	localTest2 := &testing.TestInstance{
		Name:    "local.Foo2",
		Fixture: "fixture",
		Timeout: time.Hour,
		Func:    func(ctx context.Context, s *testing.State) {},
	}
	localReg := testing.NewRegistry("bundle")
	localReg.AddTestInstance(localTest)
	localReg.AddTestInstance(localTest2)

	var env *bundletest.Env
	fixture := &testing.FixtureInstance{
		Name: "fixture",
		Impl: testfixture.New(
			testfixture.WithReset(func(ctx context.Context) error {
				return fmt.Errorf("Reset failed")
			}),
		),
	}
	remoteReg := testing.NewRegistry("bundle")
	remoteReg.AddFixtureInstance(fixture)

	env = setUpWithDefaultDUT(t, bundletest.WithLocalBundles(localReg), bundletest.WithRemoteBundles(remoteReg))
	cl := protocol.NewTestServiceClient(env.DialRemoteBundle(ctx, t, remoteReg.Name()))

	events, err := protocoltest.RunTestsRecursiveForEvents(ctx, cl, env.RunConfig(), protocoltest.WithEntityLogs())
	if err != nil {
		t.Error(err)
	}

	wantEvents := []protocol.Event{
		&protocol.EntityStartEvent{Entity: fixture.EntityProto()},
		&protocol.EntityStartEvent{Entity: localTest.EntityProto()},
		&protocol.EntityEndEvent{EntityName: localTest.Name},

		&protocol.EntityLogEvent{EntityName: fixture.Name, Text: "Fixture failed to reset: Reset failed; recovering"},
		&protocol.EntityEndEvent{EntityName: fixture.Name},
		&protocol.EntityStartEvent{Entity: fixture.EntityProto()},

		&protocol.EntityStartEvent{Entity: localTest2.EntityProto()},
		&protocol.EntityEndEvent{EntityName: localTest2.Name},
		&protocol.EntityEndEvent{EntityName: fixture.Name},
	}

	if len(events) != len(wantEvents) {
		t.Errorf("Got len %v, want %v", len(events), len(wantEvents))
	}

	if diff := cmp.Diff(events, wantEvents, protocoltest.EventCmpOpts...); diff != "" {
		t.Errorf("Events mismatch (-got +want):\n%s", diff)
	}
}

func TestTestServiceRunTests_ExternalResetSetUpFailure(t *gotesting.T) {
	ctx := context.Background()

	localTest := &testing.TestInstance{
		Name:    "local.Foo",
		Fixture: "fixture",
		Timeout: time.Hour,
		Func:    func(ctx context.Context, s *testing.State) {},
	}
	localTest2 := &testing.TestInstance{
		Name:    "local.Foo2",
		Fixture: "fixture",
		Timeout: time.Hour,
		Func:    func(ctx context.Context, s *testing.State) {},
	}
	localTest3 := &testing.TestInstance{
		Name:    "local.Foo3",
		Fixture: "fixture",
		Timeout: time.Hour,
		Func:    func(ctx context.Context, s *testing.State) {},
	}
	localReg := testing.NewRegistry("bundle")
	localReg.AddTestInstance(localTest)
	localReg.AddTestInstance(localTest2)
	localReg.AddTestInstance(localTest3)

	var setUpCount = 0
	var env *bundletest.Env
	fixture := &testing.FixtureInstance{
		Name: "fixture",
		Impl: testfixture.New(
			testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				setUpCount++
				s.Log("SetUp called")

				if setUpCount > 1 {
					s.Error("SetUp failed")
				}
				return nil
			}),
			testfixture.WithReset(func(ctx context.Context) error {
				return fmt.Errorf("Reset failed")
			}),
		),
	}
	remoteReg := testing.NewRegistry("bundle")
	remoteReg.AddFixtureInstance(fixture)

	env = setUpWithDefaultDUT(t, bundletest.WithLocalBundles(localReg), bundletest.WithRemoteBundles(remoteReg))
	cl := protocol.NewTestServiceClient(env.DialRemoteBundle(ctx, t, remoteReg.Name()))

	events, err := protocoltest.RunTestsRecursiveForEvents(ctx, cl, env.RunConfig(), protocoltest.WithEntityLogs())
	if err != nil {
		t.Error(err)
	}

	wantEvents := []protocol.Event{
		&protocol.EntityStartEvent{Entity: fixture.EntityProto()},
		&protocol.EntityLogEvent{EntityName: fixture.Name, Text: "SetUp called"},

		&protocol.EntityStartEvent{Entity: localTest.EntityProto()},
		&protocol.EntityEndEvent{EntityName: localTest.Name},

		&protocol.EntityLogEvent{EntityName: fixture.Name, Text: "Fixture failed to reset: Reset failed; recovering"},
		&protocol.EntityEndEvent{EntityName: fixture.Name},

		&protocol.EntityStartEvent{Entity: fixture.EntityProto()},
		&protocol.EntityLogEvent{EntityName: fixture.Name, Text: "SetUp called"},
		&protocol.EntityErrorEvent{EntityName: fixture.Name, Error: &protocol.Error{Reason: "SetUp failed"}},
		&protocol.EntityEndEvent{EntityName: fixture.Name},

		&protocol.EntityStartEvent{Entity: localTest2.EntityProto()},
		&protocol.EntityErrorEvent{EntityName: localTest2.Name, Error: &protocol.Error{Reason: "[Fixture failure] fixture: SetUp failed"}},
		&protocol.EntityEndEvent{EntityName: localTest2.Name},

		&protocol.EntityStartEvent{Entity: localTest3.EntityProto()},
		&protocol.EntityErrorEvent{EntityName: localTest3.Name, Error: &protocol.Error{Reason: "[Fixture failure] fixture: SetUp failed"}},
		&protocol.EntityEndEvent{EntityName: localTest3.Name},
	}

	if diff := cmp.Diff(events, wantEvents, protocoltest.EventCmpOpts...); diff != "" {
		t.Errorf("Events mismatch (-got +want):\n%s", diff)
	}
}
