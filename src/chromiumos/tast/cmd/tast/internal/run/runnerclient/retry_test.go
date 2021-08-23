// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runnerclient

import (
	"context"
	"errors"
	"testing"

	"chromiumos/tast/cmd/tast/internal/run/config"
	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
)

// runFirstTest is runTestsFunc that pretends to run the first test only. It returns remaining tests as unstarted.
func runFirstTest(ctx context.Context, patterns []string) (results []*resultsjson.Result, unstarted []string, err error) {
	if len(patterns) == 0 {
		return nil, nil, nil
	}
	return []*resultsjson.Result{{}}, patterns[1:], errors.New("failure")
}

// runFirstTestNoUnstarted is runTestsFunc that pretends to run the first test only. It returns nil as unstarted.
func runFirstTestNoUnstarted(ctx context.Context, patterns []string) (results []*resultsjson.Result, unstarted []string, err error) {
	if len(patterns) == 0 {
		return nil, nil, nil
	}
	return []*resultsjson.Result{{}}, nil, errors.New("failure")
}

// runNoTestWithUnstarted is runTestsFunc that pretends to run no test. It returns patterns as unstarted as-is.
func runNoTestWithUnstarted(ctx context.Context, patterns []string) (results []*resultsjson.Result, unstarted []string, err error) {
	if len(patterns) == 0 {
		return nil, nil, nil
	}
	return nil, patterns, errors.New("failure")
}

// beforeRetrySuccess is beforeRetryFunc that returns success without doing anything.
func beforeRetrySuccess(ctx context.Context) bool {
	return true
}

func TestRunTestsWithRetry(t *testing.T) {
	cfg := (&config.MutableConfig{
		ContinueAfterFailure: true,
	}).Freeze()

	patterns := []string{"test1", "test2", "test3"}
	results, err := runTestsWithRetry(context.Background(), cfg, patterns, runFirstTest, beforeRetrySuccess)
	if err != nil {
		t.Fatal("runTestsWithRetry: ", err)
	}
	if len(results) != len(patterns) {
		t.Errorf("runTestsWithRetry returned %d results; want %d", len(results), len(patterns))
	}
}

func TestRunTestsWithRetryNoRetry(t *testing.T) {
	cfg := (&config.MutableConfig{
		ContinueAfterFailure: false, // disable retry
	}).Freeze()

	patterns := []string{"test1", "test2", "test3"}
	results, err := runTestsWithRetry(context.Background(), cfg, patterns, runFirstTest, beforeRetrySuccess)
	if err == nil {
		t.Fatal("runTestsWithRetry succeeded unexpectedly")
	}
	if len(results) != 1 {
		t.Errorf("runTestsWithRetry returned %d results; want %d", len(results), 1)
	}
}

func TestRunTestsWithRetryNoUnstarted(t *testing.T) {
	cfg := (&config.MutableConfig{
		ContinueAfterFailure: true,
	}).Freeze()

	patterns := []string{"test1", "test2", "test3"}
	results, err := runTestsWithRetry(context.Background(), cfg, patterns, runFirstTestNoUnstarted, beforeRetrySuccess)
	if err == nil {
		t.Fatal("runTestsWithRetry succeeded unexpectedly")
	}
	if len(results) != 1 {
		t.Errorf("runTestsWithRetry returned %d results; want %d", len(results), 1)
	}
}

func TestRunTestsWithRetryStuck(t *testing.T) {
	cfg := (&config.MutableConfig{
		ContinueAfterFailure: true,
	}).Freeze()

	patterns := []string{"test1", "test2", "test3"}
	results, err := runTestsWithRetry(context.Background(), cfg, patterns, runNoTestWithUnstarted, beforeRetrySuccess)
	if err == nil {
		t.Fatal("runTestsWithRetry succeeded unexpectedly")
	}
	if len(results) != 0 {
		t.Errorf("runTestsWithRetry returned %d results; want %d", len(results), 0)
	}
}

func TestRunTestsWithRetryBeforeRetry(t *testing.T) {
	cfg := (&config.MutableConfig{
		ContinueAfterFailure: true,
	}).Freeze()

	beforeRetryFailure := func(ctx context.Context) bool {
		return false
	}

	patterns := []string{"test1", "test2", "test3"}
	results, err := runTestsWithRetry(context.Background(), cfg, patterns, runFirstTest, beforeRetryFailure)
	if err == nil {
		t.Fatal("runTestsWithRetry succeeded unexpectedly")
	}
	if len(results) != 1 {
		t.Errorf("runTestsWithRetry returned %d results; want %d", len(results), 1)
	}
}
