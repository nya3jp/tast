// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging_test

import (
	"bytes"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/logging"
)

// memorySink is a Sink that accumulates logs to an in-memory buffer.
type memorySink struct {
	mu   sync.Mutex
	msgs []string
}

func (ms *memorySink) Log(msg string) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.msgs = append(ms.msgs, msg)
}

func (ms *memorySink) Get() []string {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return append([]string(nil), ms.msgs...)
}

func TestSinkLogger(t *testing.T) {
	var sink memorySink
	logger := logging.NewSinkLogger(logging.LevelInfo, false, &sink)
	logger.Log(logging.LevelInfo, time.Time{}, "foo")
	logger.Log(logging.LevelInfo, time.Time{}, "bar\nbaz\n")

	want := []string{"foo", "bar\nbaz\n"}
	if diff := cmp.Diff(sink.Get(), want); diff != "" {
		t.Errorf("Messages mismatch (-got +want):\n%s", diff)
	}
}

func TestSinkLogger_Level(t *testing.T) {
	var sink memorySink
	logger := logging.NewSinkLogger(logging.LevelInfo, false, &sink)
	logger.Log(logging.LevelInfo, time.Time{}, "foo")
	logger.Log(logging.LevelDebug, time.Time{}, "bar")

	want := []string{"foo"}
	if diff := cmp.Diff(sink.Get(), want); diff != "" {
		t.Errorf("Messages mismatch (-got +want):\n%s", diff)
	}
}

func TestSinkLogger_Timestamp(t *testing.T) {
	var sink memorySink
	logger := logging.NewSinkLogger(logging.LevelInfo, true, &sink)
	logger.Log(logging.LevelInfo, time.Time{}, "foo")
	logger.Log(logging.LevelInfo, time.Time{}, "bar\nbaz\n")

	msgs := sink.Get()
	if len(msgs) != 2 {
		t.Fatalf("Unexpected number of messages: got %d, want 2", len(msgs))
	}

	pattern0 := regexp.MustCompile(`^\d\d\d\d-\d\d-\d\dT\d\d:\d\d:\d\d.\d\d\d\d\d\dZ foo$`)
	pattern1 := regexp.MustCompile(`^\d\d\d\d-\d\d-\d\dT\d\d:\d\d:\d\d.\d\d\d\d\d\dZ bar\nbaz\n$`)
	if !pattern0.MatchString(msgs[0]) {
		t.Fatalf("Message mismatch: got %q, want match with regexp %q", msgs[0], pattern0.String())
	}
	if !pattern1.MatchString(msgs[1]) {
		t.Fatalf("Message mismatch: got %q, want match with regexp %q", msgs[1], pattern1.String())
	}
}

func TestSinkLogger_FuncSink(t *testing.T) {
	var got []string
	sink := logging.NewFuncSink(func(msg string) {
		got = append(got, msg)
	})
	logger := logging.NewSinkLogger(logging.LevelInfo, false, sink)
	logger.Log(logging.LevelInfo, time.Time{}, "foo")
	logger.Log(logging.LevelInfo, time.Time{}, "bar\nbaz\n")

	want := []string{"foo", "bar\nbaz\n"}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Fatalf("Messages mismatch (-got +want):\n%s", diff)
	}
}

func TestSinkLogger_WriterSink(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewSinkLogger(logging.LevelInfo, false, logging.NewWriterSink(&buf))
	logger.Log(logging.LevelInfo, time.Time{}, "foo")
	logger.Log(logging.LevelInfo, time.Time{}, "bar\nbaz\n")

	const want = "foo\nbar\nbaz\n\n"
	if got := buf.String(); got != want {
		t.Fatalf("Messages mismatch: got %q, want %q", got, want)
	}
}
