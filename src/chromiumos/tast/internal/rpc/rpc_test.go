// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	gotesting "testing"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/go-cmp/cmp"
	"github.com/shirou/gopsutil/v3/process"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/fakeexec"
	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/protocol"
	"chromiumos/tast/internal/sshtest"
	"chromiumos/tast/internal/testcontext"
	"chromiumos/tast/internal/testing"
	"chromiumos/tast/internal/testingutil"
	"chromiumos/tast/internal/timing"
	"chromiumos/tast/ssh"
	"chromiumos/tast/testutil"
)

const pingUserServiceName = "tast.coretest.PingUser"

// pingUserServer is an implementation of the Ping gRPC service.
type pingUserServer struct {
	s *testing.ServiceState
	// onPing is called when Ping is called by gRPC clients.
	onPing func(context.Context, *testing.ServiceState) error
}

func (s *pingUserServer) Ping(ctx context.Context, _ *empty.Empty) (*empty.Empty, error) {
	if err := s.onPing(ctx, s.s); err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}

type pingCoreServer struct{}

func (s *pingCoreServer) Ping(ctx context.Context, _ *empty.Empty) (*empty.Empty, error) {
	return &empty.Empty{}, nil
}

// pingPair manages a local client/server pair of the Ping gRPC service.
type pingPair struct {
	UserClient protocol.PingUserClient
	CoreClient protocol.PingCoreClient
	// The server is missing here; it is implicitly owned by the background
	// goroutine that calls RunServer.

	rpcClient  *GenericClient // underlying gRPC connection
	stopServer func() error   // func to stop the gRPC server
}

// Close closes the gRPC connection and stops the gRPC server.
func (p *pingPair) Close() error {
	firstErr := p.rpcClient.Close()
	if err := p.stopServer(); firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// newPingService defines a new Ping service.
// onPing is called when Ping gRPC method is called on the server.
func newPingService(onPing func(context.Context, *testing.ServiceState) error) *testing.Service {
	return &testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			protocol.RegisterPingUserServer(srv, &pingUserServer{s, onPing})
		},
	}
}

// newPingPair starts a local client/server pair of the Ping gRPC service.
//
// It panics if it fails to start a local client/server pair. Returned pingPair
// should be closed with pingPair.Close after its use.
func newPingPair(ctx context.Context, t *gotesting.T, req *protocol.HandshakeRequest, pingSvc *testing.Service) *pingPair {
	t.Helper()

	sr, cw := io.Pipe()
	cr, sw := io.Pipe()

	stopped := make(chan error, 1)
	go func() {
		stopped <- RunServer(sr, sw, []*testing.Service{pingSvc}, func(srv *grpc.Server, req *protocol.HandshakeRequest) error {
			protocol.RegisterPingCoreServer(srv, &pingCoreServer{})
			return nil
		})
	}()
	stopServer := func() error {
		// Close the client pipes. This will let the gRPC server close the singleton
		// gRPC connection, which triggers the gRPC server to stop via PipeListener.
		cw.Close()
		cr.Close()
		return <-stopped
	}
	success := false
	defer func() {
		if !success {
			stopServer() // no error check; test has already failed
		}
	}()

	cl, err := NewClient(ctx, cr, cw, req)
	if err != nil {
		t.Fatal("newClient failed: ", err)
	}

	success = true
	return &pingPair{
		UserClient: protocol.NewPingUserClient(cl.Conn()),
		CoreClient: protocol.NewPingCoreClient(cl.Conn()),
		rpcClient:  cl,
		stopServer: stopServer,
	}
}

type channelSink struct {
	ch chan<- string
}

func newChannelSink() (*channelSink, <-chan string) {
	// Allocate an arbitrary large buffer to avoid unit tests from hanging
	// when they don't read all messages.
	ch := make(chan string, 1000)
	return &channelSink{ch: ch}, ch
}

func (s *channelSink) Log(msg string) {
	s.ch <- msg
}

func TestRPCSuccess(t *gotesting.T) {
	ctx := testcontext.WithCurrentEntity(context.Background(), &testcontext.CurrentEntity{})
	req := &protocol.HandshakeRequest{NeedUserServices: true}

	called := false
	svc := newPingService(func(context.Context, *testing.ServiceState) error {
		called = true
		return nil
	})

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close()

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps: []string{pingUserServiceName},
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err != nil {
		t.Error("Ping failed: ", err)
	}
	if !called {
		t.Error("onPing not called")
	}
}

func TestRPCFailure(t *gotesting.T) {
	ctx := testcontext.WithCurrentEntity(context.Background(), &testcontext.CurrentEntity{})
	req := &protocol.HandshakeRequest{NeedUserServices: true}

	called := false
	svc := newPingService(func(context.Context, *testing.ServiceState) error {
		called = true
		return errors.New("failure")
	})

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close()

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps: []string{pingUserServiceName},
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err == nil {
		t.Error("Ping unexpectedly succeeded")
	}
	if !called {
		t.Error("onPing not called")
	}
}

func TestRPCNotRequested(t *gotesting.T) {
	ctx := testcontext.WithCurrentEntity(context.Background(), &testcontext.CurrentEntity{})
	req := &protocol.HandshakeRequest{} // user-defined gRPC services not requested

	called := false
	svc := newPingService(func(context.Context, *testing.ServiceState) error {
		called = true
		return nil
	})

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close()

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps: []string{pingUserServiceName},
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err == nil {
		t.Error("Ping unexpectedly succeeded")
	}
	if called {
		t.Error("onPing unexpectedly called")
	}
}

func TestRPCNoCurrentEntity(t *gotesting.T) {
	ctx := testcontext.WithCurrentEntity(context.Background(), &testcontext.CurrentEntity{})
	req := &protocol.HandshakeRequest{NeedUserServices: true}

	called := false
	svc := newPingService(func(context.Context, *testing.ServiceState) error {
		called = true
		return nil
	})

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close()

	if _, err := pp.UserClient.Ping(context.Background(), &empty.Empty{}); err == nil {
		t.Error("Ping unexpectedly succeeded for a context missing CurrentEntity")
	}
	if called {
		t.Error("onPing unexpectedly called")
	}
}

func TestRPCRejectUndeclaredServices(t *gotesting.T) {
	ctx := testcontext.WithCurrentEntity(context.Background(), &testcontext.CurrentEntity{})
	req := &protocol.HandshakeRequest{NeedUserServices: true}
	svc := newPingService(func(context.Context, *testing.ServiceState) error { return nil })
	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close()

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps: []string{"foo.Bar"},
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err == nil {
		t.Error("Ping unexpectedly succeeded despite undeclared service")
	}
}

func TestRPCForwardCurrentEntity(t *gotesting.T) {
	expectedDeps := []string{"chrome", "android_p"}

	ctx := testcontext.WithCurrentEntity(context.Background(), &testcontext.CurrentEntity{})
	req := &protocol.HandshakeRequest{NeedUserServices: true}

	called := false
	var deps []string
	var depsOK bool
	svc := newPingService(func(ctx context.Context, s *testing.ServiceState) error {
		called = true
		deps, depsOK = testcontext.SoftwareDeps(ctx)
		return nil
	})

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close()

	if _, err := pp.UserClient.Ping(ctx, &empty.Empty{}); err == nil {
		t.Error("Ping unexpectedly succeeded for a context without CurrentEntity")
	}

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps:     []string{pingUserServiceName},
		HasSoftwareDeps: true,
		SoftwareDeps:    expectedDeps,
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err != nil {
		t.Error("Ping failed: ", err)
	}
	if !called {
		t.Error("onPing not called")
	} else if !depsOK {
		t.Error("SoftwareDeps unavailable")
	} else if !reflect.DeepEqual(deps, expectedDeps) {
		t.Errorf("SoftwareDeps mismatch: got %v, want %v", deps, expectedDeps)
	}
}

func TestRPCForwardLogs(t *gotesting.T) {
	const exp = "hello"

	ctx := context.Background()
	sink, logs := newChannelSink()
	ctx = logging.AttachLogger(ctx, logging.NewSinkLogger(logging.LevelDebug, false, sink))
	ctx = testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{})
	req := &protocol.HandshakeRequest{NeedUserServices: true}

	called := false
	svc := newPingService(func(ctx context.Context, s *testing.ServiceState) error {
		called = true
		logging.Debug(ctx, "world") // not delivered
		logging.Info(ctx, exp)
		return nil
	})

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close()

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps: []string{pingUserServiceName},
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err != nil {
		t.Error("Ping failed: ", err)
	}
	if !called {
		t.Error("onPing not called")
	}

	select {
	case msg := <-logs:
		if msg != exp {
			t.Errorf("Got log %q; want %q", msg, exp)
		}
	default:
		t.Error("Logs unavailable immediately on RPC completion")
	}
}

// TestRPCForwardLogsAsyncStress is a regression test for b/207577797.
// It exercises the scenario where a remote server emits a log in parallel to
// finishing a remote method call and/or the RPC connection is closed.
func TestRPCForwardLogsAsyncStress(t *gotesting.T) {
	// n is number of attempts. n=1000 takes less than one second on modern
	// machines.
	const n = 1000

	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()

			ctx := context.Background()
			ctx = testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{})
			req := &protocol.HandshakeRequest{NeedUserServices: true}

			svc := newPingService(func(ctx context.Context, s *testing.ServiceState) error {
				logging.Info(ctx, "hello")
				go logging.Info(ctx, "world") // emit asynchronously
				return nil
			})

			pp := newPingPair(ctx, t, req, svc)
			defer pp.Close()

			callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
				ServiceDeps: []string{pingUserServiceName},
			})
			if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err != nil {
				t.Error("Ping failed: ", err)
			}
		}()
	}

	wg.Wait()
}

func TestRPCForwardTiming(t *gotesting.T) {
	const stageName = "hello"

	ctx := context.Background()
	ctx = testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{})
	log := timing.NewLog()
	ctx = timing.NewContext(ctx, log)
	req := &protocol.HandshakeRequest{NeedUserServices: true}

	called := false
	svc := newPingService(func(ctx context.Context, s *testing.ServiceState) error {
		called = true
		_, st := timing.Start(ctx, stageName)
		st.End()
		return nil
	})

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close()

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps: []string{pingUserServiceName},
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err != nil {
		t.Error("Ping failed: ", err)
	}
	if !called {
		t.Error("onPing not called")
	}

	if len(log.Root.Children) != 1 || log.Root.Children[0].Name != stageName {
		b, err := json.Marshal(log)
		if err != nil {
			t.Fatal("Failed to marshal timing JSON: ", err)
		}
		t.Errorf("Unexpected timing log: got %s, want a single %q entry", string(b), stageName)
	}
}

func TestRPCPullOutDir(t *gotesting.T) {
	outDir := testutil.TempDir(t)
	defer os.RemoveAll(outDir)

	want := map[string]string{
		"a.txt":     "abc",
		"dir/b.txt": "def",
	}

	ctx := context.Background()
	ctx = testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{})
	req := &protocol.HandshakeRequest{NeedUserServices: true}

	svc := newPingService(func(ctx context.Context, s *testing.ServiceState) error {
		od, ok := testcontext.OutDir(ctx)
		if !ok {
			return errors.New("OutDir unavailable")
		}
		if od == outDir {
			return errors.Errorf("OutDir given to service must not be that on the host: %s", od)
		}
		st, err := os.Stat(od)
		if err != nil {
			return err
		}
		const mask = os.ModePerm | os.ModeSticky
		if mode := st.Mode() & mask; mode != mask {
			return errors.Errorf("wrong directory permission: got %o, want %o", mode, mask)
		}
		return testutil.WriteFiles(od, want)
	})

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close()

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps: []string{pingUserServiceName},
		OutDir:      outDir,
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err != nil {
		t.Error("Ping failed: ", err)
	}

	got, err := testutil.ReadFiles(outDir)
	if err != nil {
		t.Fatal("Failed to read output dir: ", err)
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("Directory contents mismatch (-got +want):\n%s", diff)
	}
}

func TestRPCSetVars(t *gotesting.T) {
	ctx := testcontext.WithCurrentEntity(context.Background(), &testcontext.CurrentEntity{})
	key := "var1"
	exp := "value1"
	req := &protocol.HandshakeRequest{
		NeedUserServices: true,
		BundleInitParams: &protocol.BundleInitParams{
			Vars: map[string]string{key: exp},
		},
	}

	called := false
	var value string
	ok := false
	svc := newPingService(func(ctx context.Context, s *testing.ServiceState) error {
		called = true
		value, ok = s.Var(key)
		return nil
	})
	// Set service vars in service definition.
	svc.Vars = []string{key}

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close()

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps: []string{pingUserServiceName},
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err != nil {
		t.Error("Ping failed: ", err)
	}
	if !called {
		t.Error("onPing not called")
	}
	if !ok || value != exp {
		t.Errorf("Runtime var not set for key %q: got ok %t and value %q, want %q", key, ok, value, exp)
	}
}

func TestRPCServiceScopedContext(t *gotesting.T) {
	const exp = "hello"

	ctx := context.Background()
	sink, logs := newChannelSink()
	ctx = logging.AttachLogger(ctx, logging.NewSinkLogger(logging.LevelDebug, false, sink))
	ctx = testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{})
	req := &protocol.HandshakeRequest{NeedUserServices: true}

	called := false
	svc := newPingService(func(ctx context.Context, s *testing.ServiceState) error {
		called = true
		logging.Debug(ctx, "world") // not delivered
		logging.Info(s.ServiceContext(), exp)
		return nil
	})

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close()

	callCtx := testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{
		ServiceDeps: []string{pingUserServiceName},
	})
	if _, err := pp.UserClient.Ping(callCtx, &empty.Empty{}); err != nil {
		t.Error("Ping failed: ", err)
	}
	if !called {
		t.Error("onPing not called")
	}

	if msg := <-logs; msg != exp {
		t.Errorf("Got log %q; want %q", msg, exp)
	}
}

func TestRPCExtraCoreServices(t *gotesting.T) {
	ctx := context.Background()
	req := &protocol.HandshakeRequest{NeedUserServices: false}
	svc := newPingService(nil)

	pp := newPingPair(ctx, t, req, svc)
	defer pp.Close()

	if _, err := pp.CoreClient.Ping(ctx, &empty.Empty{}); err != nil {
		t.Error("Ping failed: ", err)
	}
}

func TestRPCOverExec(t *gotesting.T) {
	ctx := context.Background()

	// Create a loopback executable providing gRPC server.
	dir, err := ioutil.TempDir("", "tast-unittest.")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "rpc-server")
	lo, err := fakeexec.CreateLoopback(path, func(_ []string, stdin io.Reader, stdout, _ io.WriteCloser) int {
		if err := RunServer(stdin, stdout, nil, func(srv *grpc.Server, req *protocol.HandshakeRequest) error {
			protocol.RegisterPingCoreServer(srv, &pingCoreServer{})
			return nil
		}); err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
			return 1
		}
		return 0
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lo.Close()

	// Connect to the server and try calling a method.
	conn, err := DialExec(ctx, path, false, &protocol.HandshakeRequest{})
	if err != nil {
		t.Fatalf("DialExec failed: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	cl := protocol.NewPingCoreClient(conn.Conn())
	if _, err := cl.Ping(ctx, &empty.Empty{}); err != nil {
		t.Error("Ping failed: ", err)
	}
}

type leakingPingServer struct{}

func (s *leakingPingServer) Ping(ctx context.Context, _ *empty.Empty) (*empty.Empty, error) {
	// Intentionally leak a subprocess.
	exec.Command("sleep", "60").Start()
	return &empty.Empty{}, nil
}

var leakingMain = fakeexec.NewAuxMain("rpc_new_session_test", func(_ struct{}) {
	if err := RunServer(os.Stdin, os.Stdout, nil, func(srv *grpc.Server, req *protocol.HandshakeRequest) error {
		protocol.RegisterPingCoreServer(srv, &leakingPingServer{})
		return nil
	}); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
})

func TestRPCOverExecNewSession(t *gotesting.T) {
	ctx := context.Background()

	params, err := leakingMain.Params(struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	restore := params.SetEnvs()
	defer restore()

	for _, newSession := range []bool{false, true} {
		t.Run(strconv.FormatBool(newSession), func(t *gotesting.T) {
			var subproc *process.Process
			func() {
				// Connect to the server and call a method.
				conn, err := DialExec(ctx, params.Executable(), newSession, &protocol.HandshakeRequest{})
				if err != nil {
					t.Fatalf("DialExec failed: %v", err)
				}
				defer func() {
					if err := conn.Close(); err != nil {
						t.Errorf("Close failed: %v", err)
					}
				}()

				// Call Ping. This will leak a subprocess.
				cl := protocol.NewPingCoreClient(conn.Conn())
				if _, err := cl.Ping(ctx, &empty.Empty{}); err != nil {
					t.Error("Ping failed: ", err)
				}

				// Find the leaked subprocess.
				procs, err := process.Processes()
				if err != nil {
					t.Fatalf("Failed to enumerate processes: %v", err)
				}
				for _, proc := range procs {
					ppid, err := proc.Ppid()
					if err == nil && int(ppid) == conn.PID() {
						subproc = proc
						break
					}
				}
				if subproc == nil {
					t.Fatal("Failed to find a leaked subprocess")
				}
			}()

			if newSession {
				// Closing rpc.SSHClient should have killed the whole session.
				// Wait some time to allow the process to exit.
				if err := testingutil.Poll(context.Background(), func(context.Context) error {
					if _, err := subproc.Status(); err != nil {
						return nil
					}
					return errors.Errorf("process %d still exists", subproc.Pid)
				}, &testingutil.PollOptions{Timeout: 10 * time.Second}); err != nil {
					t.Fatalf("Failed to wait for a leaked subprocess to exit: %v", err)
				}
			} else {
				// Leaked subprocess should be still running.
				if err := subproc.Terminate(); err != nil {
					t.Fatalf("Failed to kill the leaked subprocess: %v", err)
				}
			}
		})
	}
}

func TestRPCOverSSH(t *gotesting.T) {
	ctx := context.Background()

	// Start a fake SSH server providing gRPC server.
	td := sshtest.NewTestData(func(req *sshtest.ExecReq) {
		req.Start(true)
		if err := RunServer(req, req, nil, func(srv *grpc.Server, req *protocol.HandshakeRequest) error {
			protocol.RegisterPingCoreServer(srv, &pingCoreServer{})
			return nil
		}); err != nil {
			fmt.Fprintf(req.Stderr(), "FATAL: %v\n", err)
			req.End(1)
			return
		}
		req.End(0)
	})
	defer td.Close()

	sshConn, err := ssh.New(ctx, &ssh.Options{
		Hostname: td.Srvs[0].Addr().String(),
		KeyFile:  td.UserKeyFile,
	})
	if err != nil {
		t.Fatalf("Failed to connect to fake SSH server: %v", err)
	}
	defer sshConn.Close(ctx)

	// Connect to the server and try calling a method.
	conn, err := DialSSH(ctx, sshConn, "", &protocol.HandshakeRequest{}, false)
	if err != nil {
		t.Fatalf("DialSSH failed: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	cl := protocol.NewPingCoreClient(conn.Conn())
	if _, err := cl.Ping(ctx, &empty.Empty{}); err != nil {
		t.Error("Ping failed: ", err)
	}
}

const (
	textReady    = "ready"
	textFinished = "finished"
)

type subprocessServer struct {
	path string
}

func (s *subprocessServer) Ping(ctx context.Context, _ *empty.Empty) (*empty.Empty, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "context already canceled on entering method")
	}

	// Notify the parent process that we're in the middle of a method call.
	ioutil.WriteFile(s.path, []byte(textReady), 0666)

	// Wait for the context to be canceled.
	<-ctx.Done()

	// Notify the parent process that we're finishing the method call.
	ioutil.WriteFile(s.path, []byte(textFinished), 0666)

	return &empty.Empty{}, nil
}

var stdioMain = fakeexec.NewAuxMain("rpc_stdio_test", func(path string) {
	RunServer(os.Stdin, os.Stdout, nil, func(s *grpc.Server, req *protocol.HandshakeRequest) error {
		protocol.RegisterPingCoreServer(s, &subprocessServer{path})
		return nil
	})
})

// runStdioTestServer starts a subprocess serving subprocessServer and
// starts an asynchronous call of its Ping method.
func runStdioTestServer(t *gotesting.T) (cmd *exec.Cmd, stdin io.WriteCloser, stdout io.ReadCloser, waitReady, waitFinish func(t *gotesting.T)) {
	ctx := context.Background()

	// Create a temporary file. Is is initially empty, but a subprocess
	// writes some data to it later.
	f, err := ioutil.TempFile("", "tast-unittest.")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		f.Close()
		os.Remove(f.Name())
	})

	// Run a fake subprocess serving subprocessServer.
	params, err := stdioMain.Params(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	cmd = exec.Command(params.Executable())
	cmd.Env = append(os.Environ(), params.Envs()...)
	cmd.Stderr = os.Stderr
	stdin, err = cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err = cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})

	conn, err := NewClient(ctx, stdout, stdin, &protocol.HandshakeRequest{})
	if err != nil {
		t.Fatalf("Failed to establish gRPC connection to subprocess: %v", err)
	}
	t.Cleanup(func() {
		conn.Close()
	})

	// Make an RPC call on a goroutine.
	go func() {
		cl := protocol.NewPingCoreClient(conn.Conn())
		cl.Ping(ctx, &empty.Empty{})
	}()

	// waitText waits until f's content becomes the specified one.
	waitText := func(t *gotesting.T, want string) {
		if err := testingutil.Poll(ctx, func(ctx context.Context) error {
			b, err := ioutil.ReadFile(f.Name())
			if err != nil {
				return testingutil.PollBreak(err)
			}
			got := string(b)
			if got != want {
				return errors.Errorf("content mismatch: got %q, want %q", got, want)
			}
			return nil
		}, &testingutil.PollOptions{Timeout: 10 * time.Second}); err != nil {
			t.Fatalf("Failed to wait for subprocess write: %v", err)
		}
	}
	// waitReady waits for the subprocess to enter the gRPC method.
	waitReady = func(t *gotesting.T) { waitText(t, textReady) }
	// waitFinish wait for the subprocess to finish the gRPC method call.
	waitFinish = func(t *gotesting.T) { waitText(t, textFinished) }
	return cmd, stdin, stdout, waitReady, waitFinish
}

func TestRPCOverStdioSIGPIPE(t *gotesting.T) {
	_, stdin, stdout, waitReady, waitSuccess := runStdioTestServer(t)

	waitReady(t)

	// Close stdout of the subprocess. If the subprocess doesn't install
	// SIGPIPE handlers, writing data to stdout will cause termination.
	stdout.Close()

	// Close stdin to stop the gRPC server.
	stdin.Close()

	waitSuccess(t)
}

func TestRPCOverStdioSIGINT(t *gotesting.T) {
	cmd, _, _, waitReady, waitSuccess := runStdioTestServer(t)

	waitReady(t)

	// Send SIGINT to the subprocess. If the subprocess doesn't install
	// SIGINT handlers it will terminate immediately.
	cmd.Process.Signal(unix.SIGINT)

	waitSuccess(t)
}
