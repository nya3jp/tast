// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"context"
	"io"
	gotesting "testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/rpc"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
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

func (ff *fakeFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if ff.tearDown != nil {
		ff.tearDown(ctx, s)
	}
}

func (ff *fakeFixture) Reset(ctx context.Context) error {
	panic("Reset is not supported for remote fixtures yet.")
}

// setUpFakeFixtureService starts a fixture service server and returns a client connected to it.
// close must be called by the caller.
func setUpFakeFixtureService(ctx context.Context, t *gotesting.T) (rfcl FixtureService_RunFixtureClient, close func()) {
	t.Helper()

	sr, cw := io.Pipe()
	cr, sw := io.Pipe()

	stopped := make(chan error, 1)
	go func() {
		stopped <- RunFixtureServiceServer(sr, sw)
	}()

	close = func() { // stop the server
		cw.Close()
		cr.Close()
		if err := <-stopped; err != nil {
			t.Errorf("Server error: %v", err)
		}
	}
	success := false
	defer func() {
		if !success {
			close()
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
	return rfcl, close
}

func TestFixtureService(t *gotesting.T) {
	pushReq := &RunFixtureRequest{
		Control: &RunFixtureRequest_Push{
			Push: &RunFixturePushRequest{
				Name: "fake",
				Config: &RunFixtureConfig{
					TempDir: "/tmp",
					OutDir:  "/tmp",
					Target:  "foo@bar.baz",
				},
			},
		},
	}
	popReq := &RunFixtureRequest{
		Control: &RunFixtureRequest_Pop{
			Pop: &RunFixturePopRequest{},
		},
	}

	for _, tc := range []struct {
		name        string
		fixt        *fakeFixture
		requests    []*RunFixtureRequest
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
			requests: []*RunFixtureRequest{
				pushReq,
				popReq,
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
			requests: []*RunFixtureRequest{
				pushReq,
				popReq,
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
			rfcl, close := setUpFakeFixtureService(ctx, t)
			defer close()

			// responses reads responses from rfcl. It checks fields not suitable for exact
			// comparison (e.g. timestamp) are non-zero and fills in zero values.
			responses := func() []*RunFixtureResponse {
				var res []*RunFixtureResponse
				for {
					r, err := rfcl.Recv()
					if err == io.EOF {
						return res
					} else if err != nil {
						t.Errorf("rfcl.Recv(): %v", err)
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

			var got [][]*RunFixtureResponse
			for _, req := range tc.requests {
				rfcl.Send(req)
				got = append(got, responses())
			}

			if diff := cmp.Diff(tc.wantResults, got); diff != "" {
				t.Errorf("Results mismatch (-want +got): %v", diff)
			}
		})
	}
}

func TestFixtureServiceNoSuchFixture(t *gotesting.T) {
	restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
	defer restore()

	testing.AddFixture(&testing.Fixture{Name: "fake", Impl: &fakeFixture{}})

	ctx := context.Background()
	rfcl, close := setUpFakeFixtureService(ctx, t)
	defer close()

	if err := rfcl.Send(&RunFixtureRequest{
		Control: &RunFixtureRequest_Push{
			Push: &RunFixturePushRequest{
				Name: "noSuchFixture",
				Config: &RunFixtureConfig{
					TempDir: "/tmp",
					OutDir:  "/tmp",
					Target:  "foo@bar.baz",
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
	rfcl, close := setUpFakeFixtureService(ctx, t)
	defer close()

	if err := rfcl.Send(&RunFixtureRequest{
		Control: &RunFixtureRequest_Push{
			Push: &RunFixturePushRequest{
				Name: "fake",
				Config: &RunFixtureConfig{
					TempDir:           "/tmp",
					OutDir:            "/tmp",
					Target:            "foo@bar.baz",
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

func TestFixtureServiceRequestOrder(t *gotesting.T) {
	push := &RunFixtureRequest{
		Control: &RunFixtureRequest_Push{
			Push: &RunFixturePushRequest{
				Name: "fake",
				Config: &RunFixtureConfig{
					TempDir: "/tmp",
					OutDir:  "/tmp",
					Target:  "foo@bar.baz",
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
		name    string
		ops     []*RunFixtureRequest
		wantErr bool
	}{
		{
			name:    "pop without push",
			ops:     []*RunFixtureRequest{pop},
			wantErr: true,
		},
		{
			name:    "push twice in a row",
			ops:     []*RunFixtureRequest{push, push},
			wantErr: true,
		},
		{
			name: "push pop push pop",
			ops:  []*RunFixtureRequest{push, pop, push, pop},
		},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			restore := testing.SetGlobalRegistryForTesting(testing.NewRegistry())
			defer restore()
			testing.AddFixture(&testing.Fixture{Name: "fake", Impl: &fakeFixture{}})

			ctx := context.Background()
			rfcl, close := setUpFakeFixtureService(ctx, t)
			defer close()

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

			for i, op := range tc.ops {
				last := i == len(tc.ops)-1
				if err := rfcl.Send(op); err != nil {
					t.Fatal(err)
				}

				err := consume()
				if !last && err != nil {
					t.Fatalf("ops[%d]: %v", i, err)
				}
				if last && !tc.wantErr && err != nil && err != io.EOF {
					t.Errorf("Last request failed: %v", err)
				}
				if last && tc.wantErr && (err == nil || err == io.EOF) {
					t.Errorf("Last err = %v, want error", err)
				}
			}
		})
	}
}
