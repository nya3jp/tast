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
		t.Error("ContextOutDir unexpectedly succeeded for context without CurrentEntity")
	}

	ec := &CurrentEntity{OutDir: testOutDir}
	ctx = WithCurrentEntity(ctx, ec)

	if outDir, ok := ContextOutDir(ctx); !ok {
		t.Error("ContextOutDir failed for context with CurrentEntity")
	} else if outDir != testOutDir {
		t.Errorf("ContextOutDir = %q; want %q", outDir, testOutDir)
	}

	ec = &CurrentEntity{OutDir: ""}
	ctx = WithCurrentEntity(ctx, ec)

	if _, ok := ContextOutDir(ctx); ok {
		t.Error("ContextOutDir unexpectedly succeeded for empty OutDir")
	}
}

func TestContextSoftwareDeps(t *testing.T) {
	var testSoftwareDeps = []string{"foo", "bar"}

	ctx := context.Background()

	if _, ok := ContextSoftwareDeps(ctx); ok {
		t.Error("ContextSoftwareDeps unexpectedly succeeded for context without CurrentEntity")
	}

	if _, ok := ContextSoftwareDeps(WithCurrentEntity(ctx, &CurrentEntity{})); ok {
		t.Error("ContextSoftwareDeps unexpectedly succeeded for context without SoftwareDeps")
	}

	ec := &CurrentEntity{
		HasSoftwareDeps: true,
		SoftwareDeps:    testSoftwareDeps,
	}
	ctx = WithCurrentEntity(ctx, ec)

	if softwareDeps, ok := ContextSoftwareDeps(ctx); !ok {
		t.Error("ContextSoftwareDeps failed for context with CurrentEntity")
	} else if !reflect.DeepEqual(softwareDeps, testSoftwareDeps) {
		t.Errorf("ContextSoftwareDeps = %q; want %q", softwareDeps, testSoftwareDeps)
	}
}

func TestContextServiceDeps(t *testing.T) {
	var testServiceDeps = []string{"foo", "bar"}

	ctx := context.Background()

	if _, ok := ContextServiceDeps(ctx); ok {
		t.Error("ContextServiceDeps unexpectedly succeeded for context without CurrentEntity")
	}

	ec := &CurrentEntity{ServiceDeps: testServiceDeps}
	ctx = WithCurrentEntity(ctx, ec)

	if serviceDeps, ok := ContextServiceDeps(ctx); !ok {
		t.Error("ContextServiceDeps failed for context with CurrentEntity")
	} else if !reflect.DeepEqual(serviceDeps, testServiceDeps) {
		t.Errorf("ContextServiceDeps = %q; want %q", serviceDeps, testServiceDeps)
	}
}
