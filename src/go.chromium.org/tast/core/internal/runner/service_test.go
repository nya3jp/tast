// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	gotesting "testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"go.chromium.org/tast/core/internal/bundle/fakebundle"
	"go.chromium.org/tast/core/internal/devserver/devservertest"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/protocol/protocoltest"
	"go.chromium.org/tast/core/internal/rpc"
	"go.chromium.org/tast/core/internal/testing"
	"go.chromium.org/tast/core/testutil"
)

// startTestServer starts an in-process gRPC server and returns a connection as
// TestServiceClient. On completion of the current test, resources are released
// automatically.
func startTestServer(t *gotesting.T, params *protocol.RunnerInitParams) protocol.TestServiceClient {
	sr, cw := io.Pipe()
	cr, sw := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		Run([]string{"-rpc"}, sr, sw, io.Discard, &StaticConfig{})
	}()
	t.Cleanup(func() {
		cw.Close()
		cr.Close()
		<-done
	})

	conn, err := rpc.NewClient(context.Background(), cr, cw, &protocol.HandshakeRequest{RunnerInitParams: params})
	if err != nil {
		t.Fatalf("Failed to connect to in-process gRPC server: %v", err)
	}
	t.Cleanup(func() {
		conn.Close()
	})

	return protocol.NewTestServiceClient(conn.Conn())
}

func TestTestServerListEntities(t *gotesting.T) {
	// Create two fake bundles.
	t1 := &testing.TestInstance{Name: "pkg.Test1"}
	t2 := &testing.TestInstance{Name: "pkg.Test2"}
	f1 := &testing.FixtureInstance{Name: "fixt1"}
	f2 := &testing.FixtureInstance{Name: "fixt2"}

	reg1 := testing.NewRegistry("a")
	reg2 := testing.NewRegistry("b")
	reg1.AddTestInstance(t1)
	reg2.AddTestInstance(t2)
	reg1.AddFixtureInstance(f1)
	reg2.AddFixtureInstance(f2)

	bundleGlob := fakebundle.Install(t, reg1, reg2)

	cl := startTestServer(t, &protocol.RunnerInitParams{BundleGlob: bundleGlob})

	// Call ListEntities.
	got, err := cl.ListEntities(context.Background(), &protocol.ListEntitiesRequest{})
	if err != nil {
		t.Fatalf("ListEntities failed: %v", err)
	}

	want := &protocol.ListEntitiesResponse{
		Entities: []*protocol.ResolvedEntity{
			{Entity: f1.EntityProto()},
			{Entity: f2.EntityProto()},
			{Entity: t1.EntityProto()},
			{Entity: t2.EntityProto()},
		},
	}
	sorter := func(a, b *protocol.ResolvedEntity) bool {
		return a.GetEntity().GetName() < b.GetEntity().GetName()
	}
	if diff := cmp.Diff(got, want, protocmp.Transform(), protocmp.SortRepeated(sorter)); diff != "" {
		t.Errorf("ListEntitiesResponse mismatch (-got +want):\n%s", diff)
	}
}

func TestTestServerGlobalRuntimeVars(t *gotesting.T) {

	reg1 := testing.NewRegistry("a")
	reg2 := testing.NewRegistry("b")
	var1 := testing.NewVarString("var1", "", "description")
	reg1.AddVar(var1)
	var2 := testing.NewVarString("var2", "", "description")
	reg1.AddVar(var2)
	var3 := testing.NewVarString("var3", "", "description")
	reg2.AddVar(var3)
	var4 := testing.NewVarString("var4", "", "description")
	reg2.AddVar(var4)
	bundleGlob := fakebundle.Install(t, reg1, reg2)

	cl := startTestServer(t, &protocol.RunnerInitParams{BundleGlob: bundleGlob})

	// Call GlobalRuntimeVars.
	got, err := cl.GlobalRuntimeVars(context.Background(), &protocol.GlobalRuntimeVarsRequest{})
	if err != nil {
		t.Fatalf("GlobalRuntimeVars failed: %v", err)
	}

	want := &protocol.GlobalRuntimeVarsResponse{
		Vars: []*protocol.GlobalRuntimeVar{
			{Name: "var1"},
			{Name: "var2"},
			{Name: "var3"},
			{Name: "var4"},
		},
	}
	sorter := func(a, b *protocol.GlobalRuntimeVar) bool {
		return a.GetName() < b.GetName()
	}
	if diff := cmp.Diff(got, want, protocmp.Transform(), protocmp.SortRepeated(sorter)); diff != "" {
		t.Errorf("GlobalRuntimeVarsResponse mismatch (-got +want):\n%s", diff)
	}
}

func TestTestServerRunTests(t *gotesting.T) {
	// Create two fake bundles.
	test1 := &testing.TestInstance{
		Name:    "pkg.Test1",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}
	test2 := &testing.TestInstance{
		Name: "pkg.Test2",
		Func: func(ctx context.Context, s *testing.State) {
			s.Error("Test2 failed")
		},
		Timeout: time.Minute,
	}
	test3 := &testing.TestInstance{
		Name:    "pkg.Test3",
		Func:    func(ctx context.Context, s *testing.State) {},
		Timeout: time.Minute,
	}

	reg1 := testing.NewRegistry("bundle1")
	reg1.AddTestInstance(test1)
	reg1.AddTestInstance(test2)
	reg2 := testing.NewRegistry("bundle2")
	reg2.AddTestInstance(test3)

	bundleGlob := fakebundle.Install(t, reg1, reg2)

	cl := startTestServer(t, &protocol.RunnerInitParams{BundleGlob: bundleGlob})

	// Call RunTests.
	events, err := protocoltest.RunTestsForEvents(context.Background(), cl, &protocol.RunConfig{})
	if err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}

	wantEvents := []protocol.Event{
		&protocol.EntityStartEvent{Entity: test1.EntityProto()},
		&protocol.EntityEndEvent{EntityName: test1.Name},
		&protocol.EntityStartEvent{Entity: test2.EntityProto()},
		&protocol.EntityErrorEvent{EntityName: test2.Name, Error: &protocol.Error{Reason: "Test2 failed"}},
		&protocol.EntityEndEvent{EntityName: test2.Name},
		&protocol.EntityStartEvent{Entity: test3.EntityProto()},
		&protocol.EntityEndEvent{EntityName: test3.Name},
	}
	if diff := cmp.Diff(events, wantEvents, protocoltest.EventCmpOpts...); diff != "" {
		t.Errorf("Events mismatch (-got +want):\n%s", diff)
	}
}

func TestTestServerStreamFile(t *gotesting.T) {
	cl := startTestServer(t, nil)
	ctx := context.Background()

	dataDir := testutil.TempDir(t)
	defer os.RemoveAll(dataDir)
	src := filepath.Join(dataDir, "test.log")
	f, err := os.OpenFile(src, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		t.Fatalf("failed to create file %v: %v", src, err)
	}
	defer f.Close()

	stream, err := cl.StreamFile(ctx, &protocol.StreamFileRequest{Name: src, Offset: 0})
	if err != nil {
		t.Fatalf("StreamFile failed: %v", err)
	}

	data := []string{
		`line1\nline`,
		`2\nline3\n`,
	}
	expected := ""
	for _, d := range data {
		expected = expected + d
	}

	done := make(chan bool, 1)

	go func() {
		time.Sleep(time.Second * 2) // Simulate data not sent at once or right away.
		for _, d := range data {
			if _, err := f.Write([]byte(d)); err != nil {
				t.Errorf("failed to write data %q: %v", d, err)
			}
			time.Sleep(time.Second * 2)
		}
		<-done
	}()

	got := ""
	go func() {
		for i := range data {
			msg, err := stream.Recv()
			if err != nil {
				t.Errorf("failed to receive data %q -- data got so far %q: %v", data[i], got, err)
				return
			}
			got = got + string(msg.GetData())
		}
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("failed get all required data: got %q; want %q", got, expected)
	}
	if got != expected {
		t.Errorf("unexpect streaming data: got %q; want %q", got, expected)
	}
}

func TestTestServerStreamFileNotExist(t *gotesting.T) {
	cl := startTestServer(t, nil)
	ctx := context.Background()

	stream, err := cl.StreamFile(ctx, &protocol.StreamFileRequest{Name: "NonExistngFile.bad"})
	if err != nil {
		return
	}
	if _, err := stream.Recv(); err == nil {
		t.Fatal("StreamFile failed to return error for non-existing file")
	}
}

func TestTestServerDownloadPrivateBundlesNotFound(t *gotesting.T) {
	const artifactsURL = "gs://bucket/build/"

	// The devserver has none of the archives and responds "file not found".
	var mu sync.Mutex
	var staged []string
	ds, err := devservertest.NewServer(devservertest.StageHook(func(gsURL string) error {
		mu.Lock()
		defer mu.Unlock()
		staged = append(staged, gsURL)
		return nil
	}))
	if err != nil {
		t.Fatal("devservertest.NewServer failed: ", err)
	}
	defer ds.Close()

	stampPath := filepath.Join(t.TempDir(), "stamp")
	s := newTestServer(&StaticConfig{PrivateBundlesStampPath: stampPath}, nil, nil)
	// Missing archives are skipped as not all of them exist for all users.
	if _, err := s.DownloadPrivateBundles(t.Context(), &protocol.DownloadPrivateBundlesRequest{
		ServiceConfig:    &protocol.ServiceConfig{Devservers: []string{ds.URL}},
		BuildArtifactUrl: artifactsURL,
	}); err != nil {
		t.Fatal("DownloadPrivateBundles failed: ", err)
	}

	sort.Strings(staged)
	want := []string{artifactsURL + "tast_bundles.tar.bz2", artifactsURL + "tast_intel_bundles.tar.bz2"}
	if diff := cmp.Diff(staged, want); diff != "" {
		t.Errorf("Staged archives mismatch (-got +want):\n%s", diff)
	}
	if b, err := os.ReadFile(stampPath); err != nil {
		t.Error("Failed to read the stamp file: ", err)
	} else if got := string(b); got != artifactsURL {
		t.Errorf("Stamp file content = %q; want %q", got, artifactsURL)
	}
}

func TestTestServerDownloadPrivateBundlesFailure(t *gotesting.T) {
	ds, err := devservertest.NewServer(devservertest.Down())
	if err != nil {
		t.Fatal("devservertest.NewServer failed: ", err)
	}
	defer ds.Close()

	stampPath := filepath.Join(t.TempDir(), "stamp")
	s := newTestServer(&StaticConfig{PrivateBundlesStampPath: stampPath}, nil, nil)
	_, err = s.DownloadPrivateBundles(t.Context(), &protocol.DownloadPrivateBundlesRequest{
		ServiceConfig:    &protocol.ServiceConfig{Devservers: []string{ds.URL}},
		BuildArtifactUrl: "gs://bucket/build/",
	})
	// The failure is reported rather than skipped as a missing archive.
	const wantPrefix = "failed to download tast_bundles: "
	if err == nil || !strings.HasPrefix(err.Error(), wantPrefix) || errors.Is(err, os.ErrNotExist) {
		t.Errorf("DownloadPrivateBundles returned error %v; want non-ErrNotExist error starting with %q", err, wantPrefix)
	}
	if _, err := os.Stat(stampPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("os.Stat(%q) error = %v; want %v", stampPath, err, os.ErrNotExist)
	}
}

// openErrorClient is a devserver.Client that fails to open any file.
type openErrorClient struct {
	err error
}

func (c *openErrorClient) Open(ctx context.Context, gsURL string) (io.ReadCloser, error) {
	return nil, c.err
}

func (c *openErrorClient) Stage(ctx context.Context, gsURL string) (*url.URL, error) {
	return nil, c.err
}

func (c *openErrorClient) TearDown() error {
	return nil
}

func TestDownloadPrivateBundleOpenError(t *gotesting.T) {
	for _, tc := range []struct {
		name    string
		err     error
		wantErr bool
	}{
		{
			name: "wrapped_not_exist",
			err:  fmt.Errorf("failed to download from devserver: %w", os.ErrNotExist),
		},
		{
			name:    "other_error",
			err:     errors.New("devserver is down"),
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *gotesting.T) {
			err := downloadPrivateBundle(t.Context(), &openErrorClient{tc.err}, "gs://bucket/build/", "tast_bundles", Local)
			if tc.wantErr && !errors.Is(err, tc.err) {
				t.Errorf("downloadPrivateBundle returned error %v; want %v", err, tc.err)
			} else if !tc.wantErr && err != nil {
				t.Errorf("downloadPrivateBundle failed: %v", err)
			}
			// The error must not replace os.ErrNotExist, which would break
			// os.IsNotExist for real ENOENT errors.
			if _, err := os.Stat(filepath.Join(t.TempDir(), "missing")); !os.IsNotExist(err) {
				t.Errorf("os.IsNotExist(%v) = false; want true", err)
			}
		})
	}
}
