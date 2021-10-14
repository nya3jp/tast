// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle_test

import (
	"context"
	"io/ioutil"
	"path/filepath"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/bundle/bundletest"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/protocol/protocoltest"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/testing/testfixture"
	"chromiumos/tast/testutil"
)

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
	remoteFixture := &testing.FixtureInstance{
		Name: "remoteFixture",
		Pkg:  "fixt",
		Data: []string{"data.txt"},
		Impl: testfixture.New(
			testfixture.WithSetUp(func(ctx context.Context, s *testing.FixtState) interface{} {
				s.Log("Remote fixture SetUp")
				ioutil.WriteFile(filepath.Join(s.OutDir(), "set_up.txt"), []byte("set up"), 0644)
				b, err := ioutil.ReadFile(s.DataPath("data.txt"))
				if err != nil {
					t.Fatal(err)
				}
				if got, want := string(b), "fixture data"; got != want {
					t.Errorf("Got %v, want %v", got, want)
				}
				return nil
			}),
			testfixture.WithTearDown(func(ctx context.Context, s *testing.FixtState) {
				s.Log("Remote fixture TearDown")
				ioutil.WriteFile(filepath.Join(s.OutDir(), "tear_down.txt"), []byte("tear down"), 0644)
			}),
		),
	}

	localReg := testing.NewRegistry("bundle")
	localReg.AddTestInstance(localTest1)
	localReg.AddTestInstance(localTest2)

	remoteReg := testing.NewRegistry("bundle")
	remoteReg.AddTestInstance(remoteTest)
	remoteReg.AddFixtureInstance(remoteFixture)

	env := bundletest.SetUp(
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
		&protocol.EntityStartEvent{Entity: remoteFixture.EntityProto()},
		&protocol.EntityLogEvent{EntityName: remoteFixture.Name, Text: "Remote fixture SetUp"},
		&protocol.EntityStartEvent{Entity: localTest1.EntityProto()},
		&protocol.EntityLogEvent{EntityName: localTest1.Name, Text: "Local test 1"},
		&protocol.EntityEndEvent{EntityName: localTest1.Name},
		&protocol.EntityStartEvent{Entity: localTest2.EntityProto()},
		&protocol.EntityLogEvent{EntityName: localTest2.Name, Text: "Local test 2"},
		&protocol.EntityEndEvent{EntityName: localTest2.Name},
		&protocol.EntityLogEvent{EntityName: remoteFixture.Name, Text: "Remote fixture TearDown"},
		&protocol.EntityEndEvent{EntityName: remoteFixture.Name},
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
		"foo.Local1/out.txt":          "local1",
		"foo.Local2/out.txt":          "local2",
		"foo.Remote/out.txt":          "remote",
		"remoteFixture/set_up.txt":    "set up",
		"remoteFixture/tear_down.txt": "tear down",
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

func TestTestServiceRunTests_NoTests(t *gotesting.T) {
	env := bundletest.SetUp(t,
		bundletest.WithRemoteBundles(testing.NewRegistry("bundle")),
		bundletest.WithLocalBundles(testing.NewRegistry("bundle")),
	)
	cl := protocol.NewTestServiceClient(env.DialRemoteBundle(context.Background(), t, "bundle"))

	if _, err := protocoltest.RunTestsRecursiveForEvents(context.Background(), cl, env.RunConfig()); err != nil {
		t.Fatalf("RunTests failed for empty tests: %v", err)
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

	env := bundletest.SetUp(t, bundletest.WithLocalBundles(localReg), bundletest.WithRemoteBundles(remoteReg))
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

	env := bundletest.SetUp(t, bundletest.WithLocalBundles(localReg), bundletest.WithRemoteBundles(remoteReg))
	cl := protocol.NewTestServiceClient(env.DialRemoteBundle(context.Background(), t, remoteReg.Name()))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	events, err := protocoltest.RunTestsRecursiveForEvents(ctx, cl, env.RunConfig(), protocoltest.WithEntityLogs())
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := []protocol.Event{
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

		&protocol.EntityStartEvent{Entity: localNoSuchFixture.EntityProto()},
		&protocol.EntityErrorEvent{EntityName: localNoSuchFixture.Name, Error: &protocol.Error{Reason: `[Fixture failure] noSuchFixture: fixture "noSuchFixture" not found`}},
		&protocol.EntityEndEvent{EntityName: localNoSuchFixture.Name},
	}

	if diff := cmp.Diff(events, wantEvents, protocoltest.EventCmpOpts...); diff != "" {
		t.Errorf("Events mismatch (-got +want):\n%s", diff)
	}
}
