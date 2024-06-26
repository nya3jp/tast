// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package processor_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast/core/internal/logging"
	"go.chromium.org/tast/core/internal/logging/loggingtest"
	"go.chromium.org/tast/core/internal/minidriver/processor"
	"go.chromium.org/tast/core/internal/protocol"
	"go.chromium.org/tast/core/internal/run/fakereports"
	"go.chromium.org/tast/core/internal/run/reporting"
	"go.chromium.org/tast/core/testutil"
)

var timestampRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z `)

func stripTimestamps(text string) string {
	in := bufio.NewScanner(strings.NewReader(text))
	var out bytes.Buffer
	for in.Scan() {
		line := in.Text()
		if m := timestampRe.FindStringSubmatch(line); m != nil {
			line = line[len(m[0]):]
		}
		fmt.Fprintln(&out, line)
	}
	return out.String()
}

func TestLoggingHandler(t *testing.T) {
	resDir := t.TempDir()

	events := []protocol.Event{
		// Fixture starts.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "fixture", Type: protocol.EntityType_FIXTURE}},
		&protocol.EntityLogEvent{Time: epochpb, EntityName: "fixture", Text: "This is a log from the fixture", Level: protocol.LogLevel_INFO},
		// First test runs with 1 error.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "pkg.Test1"}},
		&protocol.EntityLogEvent{Time: epochpb, EntityName: "pkg.Test1", Text: "This is a log from the first test", Level: protocol.LogLevel_INFO},
		&protocol.EntityErrorEvent{Time: epochpb, EntityName: "pkg.Test1", Error: &protocol.Error{Reason: "Failed", Location: &protocol.ErrorLocation{File: "file.go", Line: 123, Stack: "stacktrace"}}},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "pkg.Test1"},
		// Fixture reports an error.
		&protocol.EntityErrorEvent{Time: epochpb, EntityName: "fixture", Error: &protocol.Error{Reason: "Failed", Location: &protocol.ErrorLocation{File: "fixture.go", Line: 456, Stack: "stacktrace"}}},
		// Second test runs with no error.
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "pkg.Test2"}},
		&protocol.EntityLogEvent{Time: epochpb, EntityName: "pkg.Test2", Text: "This is a log from the second test", Level: protocol.LogLevel_INFO},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "pkg.Test2"},
		// Fixture ends.
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "fixture"},
	}

	logger := loggingtest.NewLogger(t, logging.LevelDebug)
	multiplexer := logging.NewMultiLogger()

	ctx := context.Background()
	ctx = logging.AttachLogger(ctx, logger)
	ctx = logging.AttachLogger(ctx, multiplexer)

	hs := newHandlers(resDir, multiplexer, nopPull, nil, nil)
	proc := processor.New(resDir, nopDiagnose, hs, "cros")
	runProcessor(ctx, proc, events, nil)

	if err := proc.FatalError(); err != nil {
		t.Errorf("Processor had a fatal error: %v", err)
	}

	// Everything is logged via ctx.
	const wantGlobalLogs = `Started fixture fixture
[00:00:00.000] This is a log from the fixture
Started test pkg.Test1
[00:00:00.000] This is a log from the first test
[00:00:00.000] Error at file.go:123: Failed
[00:00:00.000] Stack trace:
stacktrace
Completed test pkg.Test1 in 0s with 1 error(s)
[00:00:00.000] Error at fixture.go:456: Failed
[00:00:00.000] Stack trace:
stacktrace
Started test pkg.Test2
[00:00:00.000] This is a log from the second test
Completed test pkg.Test2 in 0s with 0 error(s)
Completed fixture fixture in 0s with 1 error(s)`
	if diff := cmp.Diff(logger.String(), wantGlobalLogs); diff != "" {
		t.Errorf("Full logs mismatch (-got +want):\n%s", diff)
	}

	// Ensure that per-entity logs are saved.
	files, err := testutil.ReadFiles(resDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		path string
		want string
	}{
		{
			path: "fixtures/fixture/log.txt",
			want: `Started fixture fixture
[00:00:00.000] This is a log from the fixture
Started test pkg.Test1
[00:00:00.000] This is a log from the first test
[00:00:00.000] Error at file.go:123: Failed
[00:00:00.000] Stack trace:
stacktrace
Completed test pkg.Test1 in 0s with 1 error(s)
[00:00:00.000] Error at fixture.go:456: Failed
[00:00:00.000] Stack trace:
stacktrace
Started test pkg.Test2
[00:00:00.000] This is a log from the second test
Completed test pkg.Test2 in 0s with 0 error(s)
Completed fixture fixture in 0s with 1 error(s)
`,
		},
		{
			path: "tests/pkg.Test1/log.txt",
			want: `Started test pkg.Test1
[00:00:00.000] This is a log from the first test
[00:00:00.000] Error at file.go:123: Failed
[00:00:00.000] Stack trace:
stacktrace
Completed test pkg.Test1 in 0s with 1 error(s)
`,
		},
		{
			path: "tests/pkg.Test2/log.txt",
			want: `Started test pkg.Test2
[00:00:00.000] This is a log from the second test
Completed test pkg.Test2 in 0s with 0 error(s)
`,
		},
	} {
		got := stripTimestamps(files[tc.path])
		if diff := cmp.Diff(got, tc.want); diff != "" {
			t.Errorf("%s mismatch (-got +want)\n:%s", tc.path, diff)
		}
	}
}

func TestLoggingHandler_RPCLogs(t *testing.T) {
	resDir := t.TempDir()

	const (
		test1Log = "this is a log from the first test"
		test2Log = "this is a log from the second test"
	)
	events := []protocol.Event{
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "test1"}},
		&protocol.EntityLogEvent{Time: epochpb, EntityName: "test1", Text: test1Log, Level: protocol.LogLevel_INFO},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "test1"},
		&protocol.EntityStartEvent{Time: epochpb, Entity: &protocol.Entity{Name: "test2"}},
		&protocol.EntityLogEvent{Time: epochpb, EntityName: "test2", Text: test2Log, Level: protocol.LogLevel_INFO},
		&protocol.EntityEndEvent{Time: epochpb, EntityName: "test2"},
	}

	srv, stopFunc, addr := fakereports.Start(t, 0)
	defer stopFunc()

	client, err := reporting.NewRPCClient(context.Background(), addr)
	if err != nil {
		t.Fatalf("Failed to connect to RPC results server: %v", err)
	}
	defer func() {
		if client != nil {
			client.Close()
		}
	}()

	logger := loggingtest.NewLogger(t, logging.LevelDebug)
	multiplexer := logging.NewMultiLogger()

	ctx := context.Background()
	ctx = logging.AttachLogger(ctx, logger)
	ctx = logging.AttachLogger(ctx, multiplexer)

	hs := newHandlers(resDir, multiplexer, nopPull, nil, client)
	proc := processor.New(resDir, nopDiagnose, hs, "cros")
	runProcessor(ctx, proc, events, nil)

	if err := proc.FatalError(); err != nil {
		t.Errorf("Processor had a fatal error: %v", err)
	}

	// Ensure all log messages have been delivered to the server.
	client.Close()
	client = nil

	if got := string(srv.GetLog("test1", "tests/test1/log.txt")); !strings.Contains(got, test1Log) {
		t.Errorf("Expected log not received for test 1; got %q; should contain %q", got, test1Log)
	}
	if got := string(srv.GetLog("test2", "tests/test2/log.txt")); !strings.Contains(got, test2Log) {
		t.Errorf("Expected log not received for test 2; got %q; should contain %q", got, test2Log)
	}
	if got := string(srv.GetLog("test1", "tests/test1/log.txt")); strings.Contains(got, test2Log) {
		t.Errorf("Unexpected log found in test 1 log; got %q; should not contain %q", got, test2Log)
	}
	if got := string(srv.GetLog("test2", "tests/test2/log.txt")); strings.Contains(got, test1Log) {
		t.Errorf("Unexpected log found in test 2 log; got %q; should not contain %q", got, test1Log)
	}
}
