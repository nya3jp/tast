// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"context"
	"reflect"
	"testing"
)

func TestContextOutDir(t *testing.T) {
	const testOutDir = "/mock/outdir"

	ctx := context.Background()

	if _, ok := ContextOutDir(ctx); ok {
		t.Error("ContextOutDir unexpectedly succeeded for context without EntityContext")
	}

	ec := &EntityContext{OutDir: testOutDir}
	ctx = WithEntityContext(ctx, ec)

	if outDir, ok := ContextOutDir(ctx); !ok {
		t.Error("ContextOutDir failed for context with EntityContext")
	} else if outDir != testOutDir {
		t.Errorf("ContextOutDir = %q; want %q", outDir, testOutDir)
	}

	ec = &EntityContext{OutDir: ""}
	ctx = WithEntityContext(ctx, ec)

	if _, ok := ContextOutDir(ctx); ok {
		t.Error("ContextOutDir unexpectedly succeeded for empty OutDir")
	}
}

func TestContextSoftwareDeps(t *testing.T) {
	var testSoftwareDeps = []string{"foo", "bar"}

	ctx := context.Background()

	if _, ok := ContextSoftwareDeps(ctx); ok {
		t.Error("ContextSoftwareDeps unexpectedly succeeded for context without EntityContext")
	}

	ec := &EntityContext{SoftwareDeps: testSoftwareDeps}
	ctx = WithEntityContext(ctx, ec)

	if softwareDeps, ok := ContextSoftwareDeps(ctx); !ok {
		t.Error("ContextSoftwareDeps failed for context with EntityContext")
	} else if !reflect.DeepEqual(softwareDeps, testSoftwareDeps) {
		t.Errorf("ContextSoftwareDeps = %q; want %q", softwareDeps, testSoftwareDeps)
	}
}

func TestContextServiceDeps(t *testing.T) {
	var testServiceDeps = []string{"foo", "bar"}

	ctx := context.Background()

	if _, ok := ContextServiceDeps(ctx); ok {
		t.Error("ContextServiceDeps unexpectedly succeeded for context without EntityContext")
	}

	ec := &EntityContext{ServiceDeps: testServiceDeps}
	ctx = WithEntityContext(ctx, ec)

	if serviceDeps, ok := ContextServiceDeps(ctx); !ok {
		t.Error("ContextServiceDeps failed for context with EntityContext")
	} else if !reflect.DeepEqual(serviceDeps, testServiceDeps) {
		t.Errorf("ContextServiceDeps = %q; want %q", serviceDeps, testServiceDeps)
	}
}
