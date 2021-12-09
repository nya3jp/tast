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
	"google.golang.org/grpc"
	"google.golang.org/protobuf/testing/protocmp"

	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/sshtest"
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
func startFakeFixtureService(ctx context.Context, t *gotesting.T, reg *testing.Registry) (rfcl protocol.FixtureService_RunFixtureClient, stop func()) {
	t.Helper()

	sr, cw := io.Pipe()
	cr, sw := io.Pipe()

	stopped := make(chan error, 1)
	go func() {
		stopped <- rpc.RunServer(sr, sw, nil, func(srv *grpc.Server, req *protocol.HandshakeRequest) error {
			registerFixtureService(srv, reg)
			return nil
		})
	}()

	var stopFunc []func()
	stop = func() {
		for i := len(stopFunc) - 1; i >= 0; i-- {
			stopFunc[i]()
		}
	}
	success := false
	defer func() {
		if !success {
			stop()
		}
	}()

	stopFunc = append(stopFunc, func() { // stop the server
		cw.Close()
		cr.Close()
		if err := <-stopped; err != nil {
			t.Errorf("Server error: %v", err)
		}
	})

	rpcCL, err := rpc.NewClient(ctx, cr, cw, &protocol.HandshakeRequest{
		NeedUserServices: false,
	})
	if err != nil {
		t.Fatalf("rpc.NewClient: %v", err)
	}
	stopFunc = append(stopFunc, func() {
		if err := rpcCL.Close(); err != nil {
			t.Errorf("rpcCL.Close(): %v", err)
		}
	})
	cl := protocol.NewFixtureServiceClient(rpcCL.Conn())

	rfcl, err = cl.RunFixture(ctx)
	if err != nil {
		t.Fatal(err)
	}

	stopFunc = append(stopFunc, func() {
		// Make sure the server code finishes. No error check; tests may
		// already have called it.
		rfcl.CloseSend()
		for {
			if _, err := rfcl.Recv(); err != nil {
				return
			}
		}
	})

	success = true
	return rfcl, stop
}

func TestFixtureServiceResponses(t *gotesting.T) {
	td := sshtest.NewTestData(nil)
	defer td.Close()

	requests := []*protocol.RunFixtureRequest{
		{
			Control: &protocol.RunFixtureRequest_Push{
				Push: &protocol.RunFixturePushRequest{
					Name: "fake",
					Config: &protocol.RunFixtureConfig{
						ConnectionSpec: td.Srvs[0].Addr().String(),
						KeyFile:        td.UserKeyFile,
					},
				},
			},
		},
		{
			Control: &protocol.RunFixtureRequest_Pop{
				Pop: &protocol.RunFixturePopRequest{},
			},
		},
	}

	for _, tc := range []struct {
		name        string
		fixt        *fakeFixture
		wantResults [][]*protocol.RunFixtureResponse
	}{
		{
			name: "success",
			fixt: &fakeFixture{
				setUp: func(ctx context.Context, s *testing.FixtState) interface{} {
					s.Log("SetUp")
					logging.Info(ctx, "SetUp context log")
					return nil
				},
				tearDown: func(ctx context.Context, s *testing.FixtState) {
					s.Log("TearDown")
				},
			},
			wantResults: [][]*protocol.RunFixtureResponse{
				{
					{Control: &protocol.RunFixtureResponse_Log{Log: "SetUp"}},
					{Control: &protocol.RunFixtureResponse_Log{Log: "SetUp context log"}},
					{Control: &protocol.RunFixtureResponse_RequestDone{RequestDone: &empty.Empty{}}},
				}, {
					{Control: &protocol.RunFixtureResponse_Log{Log: "TearDown"}},
					{Control: &protocol.RunFixtureResponse_RequestDone{RequestDone: &empty.Empty{}}},
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
					t.Error("TearDown called unexpectedly")
				},
			},
			wantResults: [][]*protocol.RunFixtureResponse{
				{
					{Control: &protocol.RunFixtureResponse_Error{Error: &protocol.RunFixtureError{Reason: "Panic: SetUp panic"}}},
					{Control: &protocol.RunFixtureResponse_RequestDone{RequestDone: &empty.Empty{}}},
				},
				{
					{Control: &protocol.RunFixtureResponse_RequestDone{RequestDone: &empty.Empty{}}},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			reg := testing.NewRegistry("bundle")

			reg.AddFixtureInstance(&testing.FixtureInstance{Name: "fake", Impl: tc.fixt})

			ctx := context.Background()
			rfcl, stop := startFakeFixtureService(ctx, t, reg)
			defer stop()

			// responses reads responses from rfcl. It checks fields not suitable for exact
			// comparison (e.g. timestamp) are non-zero and fills in zero values.
			responses := func() []*protocol.RunFixtureResponse {
				var res []*protocol.RunFixtureResponse
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
					case *protocol.RunFixtureResponse_RequestDone:
						return res
					case *protocol.RunFixtureResponse_Error:
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

			var got [][]*protocol.RunFixtureResponse
			for _, req := range requests {
				rfcl.Send(req)
				got = append(got, responses())
			}
			if diff := cmp.Diff(got, tc.wantResults, protocmp.Transform()); diff != "" {
				t.Errorf("Results mismatch (-got +want):\n%v", diff)
			}

			if err := rfcl.CloseSend(); err != nil {
				t.Errorf("rfcl.CloseSend() = %v, want nil", err)
			}
			if r, err := rfcl.Recv(); err != io.EOF {
				t.Errorf("rfcl.Recv() = %v, %v, want EOF", r, err)
			}
		})
	}
}

func TestFixtureServiceParameters(t *gotesting.T) {
	td := sshtest.NewTestData(nil)
	defer td.Close()

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)

	cfg := &protocol.RunFixtureConfig{
		TempDir:        filepath.Join(tmpDir, "tmp"),
		ConnectionSpec: td.Srvs[0].Addr().String(),
		KeyFile:        td.UserKeyFile,

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

	reg := testing.NewRegistry("bundle")

	reg.AddFixtureInstance(&testing.FixtureInstance{
		Name:         "fake",
		Vars:         []string{"var"},
		SetUpTimeout: time.Second,
		Impl: &fakeFixture{
			setUp: func(ctx context.Context, s *testing.FixtState) interface{} {
				if ctx.Err() != nil {
					t.Errorf("ctx.Err() = %v", ctx.Err())
				}
				if got, want := testing.ExtractLocalBundleDir(s.RPCHint()), cfg.LocalBundleDir; got != want {
					t.Errorf("LocalBundleDir = %v, want %v", got, want)
				}
				if got, want := testing.ExtractTestVars(s.RPCHint()), cfg.TestVars; !reflect.DeepEqual(got, want) {
					t.Errorf("TestVars = %v, want %v", got, want)
				}
				if !s.DUT().Connected(ctx) {
					t.Error("s.DUT().Connected() = false, want true")
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
		},
	})

	ctx := context.Background()
	rfcl, stop := startFakeFixtureService(ctx, t, reg)
	defer stop()

	if err := rfcl.Send(&protocol.RunFixtureRequest{
		Control: &protocol.RunFixtureRequest_Push{
			Push: &protocol.RunFixturePushRequest{
				Name:   "fake",
				Config: cfg,
			},
		},
	}); err != nil {
		t.Fatal("rfcl.Send():", err)
	}

	got, err := rfcl.Recv()
	if err != nil {
		t.Fatal("rfcl.Recv():", err)
	}
	if got.GetRequestDone() == nil {
		t.Errorf("Got response %v, want RequestDone", got)
	}

	if _, err := os.Stat(cfg.TempDir); err != nil {
		t.Errorf("Non-empty cfg.TempDir should not be removed; os.Stat(%v): %v", cfg.TempDir, err)
	}
}

func TestFixtureServiceDefaultTempDir(t *gotesting.T) {
	td := sshtest.NewTestData(nil)
	defer td.Close()

	reg := testing.NewRegistry("bundle")

	// If TempDir is not set, fixture service should create a temporary
	// directory for fixtures to use, and remove it after the pop operation.
	cfg := &protocol.RunFixtureConfig{
		TempDir:        "",
		ConnectionSpec: td.Srvs[0].Addr().String(),
		KeyFile:        td.UserKeyFile,
	}

	origTempDir := os.TempDir()
	var setUpTempDir string
	var tearDownTempDir string
	reg.AddFixtureInstance(&testing.FixtureInstance{
		Name: "fake",
		Impl: &fakeFixture{
			setUp: func(ctx context.Context, s *testing.FixtState) interface{} {
				setUpTempDir = os.TempDir()
				return nil
			},
			tearDown: func(ctx context.Context, s *testing.FixtState) {
				tearDownTempDir = os.TempDir()
			},
		},
	})

	rfcl, stop := startFakeFixtureService(context.Background(), t, reg)
	defer stop()

	if err := rfcl.Send(&protocol.RunFixtureRequest{
		Control: &protocol.RunFixtureRequest_Push{
			Push: &protocol.RunFixturePushRequest{
				Name:   "fake",
				Config: cfg,
			},
		},
	}); err != nil {
		t.Fatal("rfcl.Send(push):", err)
	}

	if res, err := rfcl.Recv(); err != nil {
		t.Fatal("push; rfcl.Recv():", err)
	} else if res.GetRequestDone() == nil {
		t.Fatalf("push; rfcl.Recv() = %v, want RequestDone", res)
	}

	if err := rfcl.Send(&protocol.RunFixtureRequest{
		Control: &protocol.RunFixtureRequest_Pop{
			Pop: &protocol.RunFixturePopRequest{},
		},
	}); err != nil {
		t.Fatal("rfcl.Send(pop):", err)
	}

	if res, err := rfcl.Recv(); err != nil {
		t.Fatal("pop; rfcl.Recv():", err)
	} else if res.GetRequestDone() == nil {
		t.Fatalf("pop; rfcl.Recv() = %v, want RequestDone", res)
	}

	if d := os.TempDir(); d != origTempDir {
		t.Errorf("os.TempDir() after pop = %v, want %v", d, origTempDir)
	}
	if setUpTempDir == "" || setUpTempDir == origTempDir {
		t.Errorf("os.TempDir() in SetUp = %v; originally %v", setUpTempDir, origTempDir)
	}
	if tearDownTempDir == "" || tearDownTempDir == origTempDir {
		t.Errorf("os.TempDir() in TearDown = %v; originally %v", tearDownTempDir, origTempDir)
	}
	if _, err := os.Stat(setUpTempDir); !os.IsNotExist(err) {
		t.Errorf("setUpTempDir not removed; os.Stat(%q) = %v, want not exist", setUpTempDir, err)
	}

	if err := rfcl.CloseSend(); err != nil {
		t.Fatal(err)
	}
	if r, err := rfcl.Recv(); err != io.EOF {
		t.Fatalf("last rfcl.Recv() = %v, %v, want EOF", r, err)
	}
}

func TestFixtureServiceNoSuchFixture(t *gotesting.T) {
	reg := testing.NewRegistry("bundle")

	rfcl, stop := startFakeFixtureService(context.Background(), t, reg)
	defer stop()

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)
	td := sshtest.NewTestData(nil)
	defer td.Close()

	if err := rfcl.Send(&protocol.RunFixtureRequest{
		Control: &protocol.RunFixtureRequest_Push{
			Push: &protocol.RunFixturePushRequest{
				Name: "noSuchFixture",
				Config: &protocol.RunFixtureConfig{
					OutDir:         tmpDir,
					ConnectionSpec: td.Srvs[0].Addr().String(),
					KeyFile:        td.UserKeyFile,
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
	reg := testing.NewRegistry("bundle")

	c := make(chan struct{})
	defer close(c)

	reg.AddFixtureInstance(&testing.FixtureInstance{Name: "fake", Impl: &fakeFixture{
		setUp: func(context.Context, *testing.FixtState) interface{} {
			<-c
			return nil
		},
	}})

	ctx := context.Background()
	rfcl, stop := startFakeFixtureService(ctx, t, reg)
	defer stop()

	tmpDir := testutil.TempDir(t)
	defer os.RemoveAll(tmpDir)
	td := sshtest.NewTestData(nil)
	defer td.Close()

	if err := rfcl.Send(&protocol.RunFixtureRequest{
		Control: &protocol.RunFixtureRequest_Push{
			Push: &protocol.RunFixturePushRequest{
				Name: "fake",
				Config: &protocol.RunFixtureConfig{
					TempDir:           tmpDir,
					OutDir:            tmpDir,
					ConnectionSpec:    td.Srvs[0].Addr().String(),
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
	reg := testing.NewRegistry("bundle")
	reg.AddFixtureInstance(&testing.FixtureInstance{Name: "fake", Impl: &fakeFixture{}})

	td := sshtest.NewTestData(nil)
	defer td.Close()

	push := &protocol.RunFixtureRequest{
		Control: &protocol.RunFixtureRequest_Push{
			Push: &protocol.RunFixturePushRequest{
				Name: "fake",
				Config: &protocol.RunFixtureConfig{
					ConnectionSpec: td.Srvs[0].Addr().String(),
					KeyFile:        td.UserKeyFile,
				},
			},
		},
	}
	pop := &protocol.RunFixtureRequest{
		Control: &protocol.RunFixtureRequest_Pop{
			Pop: &protocol.RunFixturePopRequest{},
		},
	}

	for _, tc := range []struct {
		name    string
		ops     []*protocol.RunFixtureRequest
		wantErr bool
	}{
		{
			name:    "pop without push",
			ops:     []*protocol.RunFixtureRequest{pop},
			wantErr: true,
		},
		{
			name:    "push without pop",
			ops:     []*protocol.RunFixtureRequest{push},
			wantErr: true,
		},
		{
			name:    "push pop pop",
			ops:     []*protocol.RunFixtureRequest{push, pop, pop},
			wantErr: true,
		},
		{
			name: "push pop push pop",
			ops:  []*protocol.RunFixtureRequest{push, pop, push, pop},
		},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			ctx := context.Background()
			rfcl, stop := startFakeFixtureService(ctx, t, reg)
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

			var gotErr error
			for i, op := range tc.ops {
				if gotErr != nil {
					t.Fatalf("ops[%d] failed: %v", i-1, gotErr)
				}

				if err := rfcl.Send(op); err != nil {
					t.Fatalf("i = %d, rfcl.Send(): %v", i, err)
				}
				gotErr = consume()
			}
			if err := rfcl.CloseSend(); err != nil {
				t.Fatalf("rfcl.CloseSend(): %v", err)
			}
			if r, err := rfcl.Recv(); err == nil {
				t.Fatalf("rfcl.Recv() = %v, want EOF", r)
			} else if err != io.EOF && gotErr == nil {
				gotErr = err
			}

			if tc.wantErr && gotErr == nil {
				t.Errorf("Last err = %v, want real error", gotErr)
			}
			if !tc.wantErr && gotErr != nil {
				t.Errorf("Got error, want nil: %v", gotErr)
			}
		})
	}
}
