// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"reflect"
	"testing"
)

func TestContextLog(t *testing.T) {
	const testLog = "foo"

	ctx := context.Background()

	// ContextLog does nothing for contexts without TestContext.
	ContextLog(ctx, testLog)

	var logs []string
	tc := &TestContext{
		Logger: func(msg string) {
			logs = append(logs, testLog)
		},
	}
	ctx = WithTestContext(ctx, tc)

	ContextLog(ctx, testLog)

	if exp := []string{testLog}; !reflect.DeepEqual(logs, exp) {
		t.Errorf("ContextLog did not work as expected: got %v, want %v", logs, exp)
	}
}

func TestContextOutDir(t *testing.T) {
	const testOutDir = "/mock/outdir"

	ctx := context.Background()

	if _, ok := ContextOutDir(ctx); ok {
		t.Error("ContextOutDir unexpectedly succeeded for context without TestContext")
	}

	tc := &TestContext{OutDir: testOutDir}
	ctx = WithTestContext(ctx, tc)

	if outDir, ok := ContextOutDir(ctx); !ok {
		t.Error("ContextOutDir failed for context with TestContext")
	} else if outDir != testOutDir {
		t.Errorf("ContextOutDir = %q; want %q", outDir, testOutDir)
	}
}

func TestContextSoftwareDeps(t *testing.T) {
	var testSoftwareDeps = []string{"foo", "bar"}

	ctx := context.Background()

	if _, ok := ContextSoftwareDeps(ctx); ok {
		t.Error("ContextSoftwareDeps unexpectedly succeeded for context without TestContext")
	}

	tc := &TestContext{SoftwareDeps: testSoftwareDeps}
	ctx = WithTestContext(ctx, tc)

	if softwareDeps, ok := ContextSoftwareDeps(ctx); !ok {
		t.Error("ContextSoftwareDeps failed for context with TestContext")
	} else if !reflect.DeepEqual(softwareDeps, testSoftwareDeps) {
		t.Errorf("ContextSoftwareDeps = %q; want %q", softwareDeps, testSoftwareDeps)
	}
}
