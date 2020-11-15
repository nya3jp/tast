// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	gotesting "testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/sshtest"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/testutil"
)

type fakeFixture struct {
	setUp    func(ctx context.Context, s *testing.FixtState) interface{}
	tearDown func(ctx context.Context, s *testing.FixtState)
}

var _ testing.FixtureImpl = (*fakeFixture)(nil)

func (ff *fakeFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if ff.setUp != nil {
		return ff.setUp(ctx, s)
	}
	return nil
}

func (ff *fakeFixture) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (ff *fakeFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

func (ff *fakeFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if ff.tearDown != nil {
		ff.tearDown(ctx, s)
	}
}

func (ff *fakeFixture) Reset(ctx context.Context) error {
	panic("Reset is not supported for remote fixtures yet.")
}

// startFakeFixtureService starts a fixture service server and returns a client connected to it.
// stop must be called by the caller.
func startFakeFixtureService(ctx context.Context, t *gotesting.T) (rfcl FixtureService_RunFixtureClient, stop func()) {
	t.Helper()

	sr, cw := io.Pipe()
	cr, sw := io.Pipe()

	stopped := make(chan error, 1)
	go func() {
		stopped <- RunFixtureServiceServer(sr, sw)
	}()

	stop = func() { // stop the server
		cw.Close()
		cr.Close()
		if err := <-stopped; err != nil {
			t.Errorf("Server error: %v", err)
		}
	}
	success := false
	defer func() {
		if !success {
			stop()
		}
	}()

	conn, err := rpc.NewPipeClientConn(ctx, cr, cw)
	if err != nil {
		t.Fatal(err)
	}
	cl := NewFixtureServiceClient(conn)

	rfcl, err = cl.RunFixture(ctx)
	if err != nil {
		t.Fatal(err)
	}
	success = true
	return rfcl, stop
}

func TestFixtureServiceResponses(t *gotesting.T) {
	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	td := sshtest.NewTestData(userKey, hostKey, nil)
	defer td.Close()

	for _, tc := range []struct {
		name        string
		fixt        *fakeFixture
		wantResults [][]*RunFixtureResponse
	}{
		{
			name: "success",
			fixt: &fakeFixture{
				setUp: func(ctx context.Context, s *testing.FixtState) interface{} {
					s.Log("SetUp")
					testcontext.Log(ctx, "SetUp context log")
					return nil
				},
				tearDown: func(ctx context.Context, s *testing.FixtState) {
					s.Log("TearDown")
				},
			},
			wantResults: [][]*RunFixtureResponse{
				{
					{Control: &RunFixtureResponse_Log{Log: "SetUp"}},
					{Control: &RunFixtureResponse_Log{Log: "SetUp context log"}},
					{Control: &RunFixtureResponse_RequestDone{RequestDone: &empty.Empty{}}},
				}, {
					{Control: &RunFixtureResponse_Log{Log: "TearDown"}},
					{Control: &RunFixtureResponse_RequestDone{RequestDone: &empty.Empty{}}},
				},
			},
		},
		{
			name: "panic",
			fixt: &fakeFixture{
				setUp: func(ctx context.Context, s *testing.FixtState) interface{} {
					panic("SetUp panic")
				},
				tearDown: func(ctx context.Context, s *testing.FixtState) {
					t.Error("Should not be called")
				},
			},

			wantResults: [][]*RunFixtureResponse{
				{
					{Control: &RunFixtureResponse_Error{Error: &RunFixtureError{Reason: "Panic: SetUp panic"}}},
					{Control: &RunFixtureResponse_RequestDone{RequestDone: &empty.Empty{}}},
				},
				{
					{Control: &RunFixtureResponse_RequestDone{RequestDone: &empty.Empty{}}},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
			defer restore()

			testing.AddFixture(&testing.Fixture{Name: "fake", Impl: tc.fixt})

			ctx := context.Background()
			rfcl, stop := startFakeFixtureService(ctx, t)
			defer stop()

			// responses reads responses from rfcl. It checks fields not suitable for exact
			// comparison (e.g. timestamp) are non-zero and fills in zero values.
			responses := func() []*RunFixtureResponse {
				var res []*RunFixtureResponse
				for {
					r, err := rfcl.Recv()
					if err == io.EOF {
						return res
					} else if err != nil {
						t.Fatalf("rfcl.Recv(): %v", err)
						return nil
					}

					if ts := r.Timestamp; ts.GetNanos() == 0 && ts.GetSeconds() == 0 {
						t.Fatalf("r.Timestamp = 0, want non-zero")
					}
					r.Timestamp = nil

					res = append(res, r)
					switch x := r.Control.(type) {
					case *RunFixtureResponse_RequestDone:
						return res
					case *RunFixtureResponse_Error:
						if x.Error.File == "" {
							t.Fatalf(`Error.File = "", want file path`)
						}
						x.Error.File = ""
						if x.Error.Line == 0 {
							t.Fatalf(`Error.Line = 0, want line number`)
						}
						x.Error.Line = 0
						if x.Error.Stack == "" {
							t.Fatalf(`Error.Stack = "", want stack trace`)
						}
						x.Error.Stack = ""
					}
				}
			}

			requests := []*RunFixtureRequest{
				{
					Control: &RunFixtureRequest_Push{
						Push: &RunFixturePushRequest{
							Name: "fake",
							Config: &RunFixtureConfig{
								Target:  td.Srv.Addr().String(),
								KeyFile: td.UserKeyFile,
							},
						},
					},
				},
				{
					Control: &RunFixtureRequest_Pop{
						Pop: &RunFixturePopRequest{},
					},
				},
			}

			var got [][]*RunFixtureResponse
			for _, req := range requests {
				rfcl.Send(req)
				got = append(got, responses())
			}
			if err := rfcl.CloseSend(); err != nil {
				t.Errorf("rfcl.CloseSend() = %v, want nil", err)
			}

			if diff := cmp.Diff(got, tc.wantResults); diff != "" {
				t.Errorf("Results mismatch (-got +want): %v", diff)
			}
		})
	}
}

func TestFixtureServiceParameters(t *gotesting.T) {
	td := sshtest.NewTestData(userKey, hostKey, func(*sshtest.ExecReq) {})
	defer td.Close()

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	cfg := &RunFixtureConfig{
		TempDir: filepath.Join(tmpDir, "tmp"),
		Target:  td.Srv.Addr().String(),
		KeyFile: td.UserKeyFile,

		OutDir:         filepath.Join(tmpDir, "out"),
		TestVars:       map[string]string{"var": "value"},
		LocalBundleDir: "/bogus/bundle",

		CheckSoftwareDeps:           true,
		AvailableSoftwareFeatures:   []string{"valid"},
		UnavailableSoftwareFeatures: []string{"missing"},

		// TODO(oka): Test Devservers and DataDir after Fixture.Data is implemented.
		// TODO(oka): Test features after Fixture.*Deps are implemented.
		// TODO(oka): Consider testing TlwServer, DutName, BuildArtifactsUrl and DownloadMode.
	}

	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()

	testing.AddFixture(&testing.Fixture{
		Name: "fake",
		Vars: []string{"var"},
		Impl: &fakeFixture{
			setUp: func(ctx context.Context, s *testing.FixtState) interface{} {
				if s.DUT() == nil {
					t.Error("s.DUT() = nil, want non-nil")
				}
				if got, want := testing.ExtractLocalBundleDir(s.RPCHint()), cfg.LocalBundleDir; got != want {
					t.Errorf("LocalBundleDir = %v, want %v", got, want)
				}
				if got, want := testing.ExtractTestVars(s.RPCHint()), cfg.TestVars; !reflect.DeepEqual(got, want) {
					t.Errorf("TestVars = %v, want %v", got, want)
				}
				if got, want := s.OutDir(), filepath.Join(cfg.OutDir, "fake"); got != want {
					t.Errorf("s.OutDir() = %v, want %v", got, want)
				}
				if got, want := os.TempDir(), cfg.TempDir; got != want {
					t.Errorf("os.TempDir() = %s, want %s", got, want)
				}
				if got, want := s.RequiredVar("var"), "value"; got != want {
					t.Errorf(`s.RequiredVar("var") = %s, want %s`, got, want)
				}
				return nil
			},
			tearDown: func(ctx context.Context, s *testing.FixtState) {
				t.Error("Should not be called")
			},
		},
	})

	ctx := context.Background()
	rfcl, stop := startFakeFixtureService(ctx, t)
	defer stop()

	if err := rfcl.Send(&RunFixtureRequest{
		Control: &RunFixtureRequest_Push{
			Push: &RunFixturePushRequest{
				Name:   "fake",
				Config: cfg,
			},
		},
	}); err != nil {
		t.Fatal("rfcl.Send(): ", err)
	}

	got, err := rfcl.Recv()
	if err != nil {
		t.Fatal("rfcl.Recv(): ", err)
	}
	if _, ok := got.Control.(*RunFixtureResponse_RequestDone); !ok {
		t.Errorf("Unexpected response: %v", got)
	}
}

func TestFixtureServiceNoSuchFixture(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()

	testing.AddFixture(&testing.Fixture{Name: "fake", Impl: &fakeFixture{}})

	ctx := context.Background()
	rfcl, stop := startFakeFixtureService(ctx, t)
	defer stop()

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)
	td := sshtest.NewTestData(userKey, hostKey, nil)
	defer td.Close()

	if err := rfcl.Send(&RunFixtureRequest{
		Control: &RunFixtureRequest_Push{
			Push: &RunFixturePushRequest{
				Name: "noSuchFixture",
				Config: &RunFixtureConfig{
					OutDir:  tmpDir,
					Target:  td.Srv.Addr().String(),
					KeyFile: td.UserKeyFile,
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := rfcl.Recv(); err == nil || err == io.EOF {
		t.Errorf("rfcl.Recv() = %v, want no such fixture error", err)
	}
}

func TestFixtureServiceTimeout(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()

	c := make(chan struct{})
	defer close(c)

	testing.AddFixture(&testing.Fixture{Name: "fake", Impl: &fakeFixture{
		setUp: func(context.Context, *testing.FixtState) interface{} {
			<-c
			return nil
		},
	}})

	ctx := context.Background()
	rfcl, stop := startFakeFixtureService(ctx, t)
	defer stop()

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)
	td := sshtest.NewTestData(userKey, hostKey, nil)
	defer td.Close()

	if err := rfcl.Send(&RunFixtureRequest{
		Control: &RunFixtureRequest_Push{
			Push: &RunFixturePushRequest{
				Name: "fake",
				Config: &RunFixtureConfig{
					TempDir:           tmpDir,
					OutDir:            tmpDir,
					Target:            td.Srv.Addr().String(),
					KeyFile:           td.UserKeyFile,
					CustomGracePeriod: ptypes.DurationProto(time.Millisecond),
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := rfcl.Recv(); err == nil || err == io.EOF {
		t.Errorf("rfcl.Recv() = %v, want fixture timeout error", err)
	}
}

func TestFixtureServiceWrongRequestOrder(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()
	testing.AddFixture(&testing.Fixture{Name: "fake", Impl: &fakeFixture{}})

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)
	td := sshtest.NewTestData(userKey, hostKey, nil)
	defer td.Close()

	push := &RunFixtureRequest{
		Control: &RunFixtureRequest_Push{
			Push: &RunFixturePushRequest{
				Name: "fake",
				Config: &RunFixtureConfig{
					TempDir: tmpDir,
					OutDir:  tmpDir,
					Target:  td.Srv.Addr().String(),
					KeyFile: td.UserKeyFile,
				},
			},
		},
	}
	pop := &RunFixtureRequest{
		Control: &RunFixtureRequest_Pop{
			Pop: &RunFixturePopRequest{},
		},
	}

	for _, tc := range []struct {
		name string
		ops  []*RunFixtureRequest
	}{
		{
			name: "pop without push",
			ops:  []*RunFixtureRequest{pop},
		},
		{
			name: "push twice",
			ops:  []*RunFixtureRequest{push, push},
		},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			ctx := context.Background()
			rfcl, stop := startFakeFixtureService(ctx, t)
			defer stop()

			consume := func() error {
				for {
					res, err := rfcl.Recv()
					if err != nil {
						return err
					}
					if x := res.GetRequestDone(); x != nil {
						return nil
					}
				}
			}

			var lastErr error
			for i, op := range tc.ops {
				if lastErr != nil {
					t.Fatalf("ops[%d] failed: %v", i-1, lastErr)
				}

				if err := rfcl.Send(op); err != nil {
					t.Fatalf("i = %d, rfcl.Send(): %v", i, err)
				}

				lastErr = consume()
			}
			if lastErr == nil || lastErr == io.EOF {
				t.Errorf("Last err = %v, want real error", lastErr)
			}
		})
	}
}
