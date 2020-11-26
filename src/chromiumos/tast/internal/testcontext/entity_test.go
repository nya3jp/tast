// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testcontext

import (
	"context"
	"reflect"
	"testing"
)

func TestOutDir(t *testing.T) {
	const testOutDir = "/mock/outdir"

	ctx := context.Background()

	if _, ok := OutDir(ctx); ok {
		t.Error("OutDir unexpectedly succeeded for context without CurrentEntity")
	}

	ec := &CurrentEntity{OutDir: testOutDir}
	ctx = WithCurrentEntity(ctx, ec)

	if outDir, ok := OutDir(ctx); !ok {
		t.Error("OutDir failed for context with CurrentEntity")
	} else if outDir != testOutDir {
		t.Errorf("OutDir = %q; want %q", outDir, testOutDir)
	}

	ec = &CurrentEntity{OutDir: ""}
	ctx = WithCurrentEntity(ctx, ec)

	if _, ok := OutDir(ctx); ok {
		t.Error("OutDir unexpectedly succeeded for empty OutDir")
	}
}

func TestSoftwareDeps(t *testing.T) {
	var testSoftwareDeps = []string{"foo", "bar"}

	ctx := context.Background()

	if _, ok := SoftwareDeps(ctx); ok {
		t.Error("SoftwareDeps unexpectedly succeeded for context without CurrentEntity")
	}

	if _, ok := SoftwareDeps(WithCurrentEntity(ctx, &CurrentEntity{})); ok {
		t.Error("SoftwareDeps unexpectedly succeeded for context without SoftwareDeps")
	}

	ec := &CurrentEntity{
		HasSoftwareDeps: true,
		SoftwareDeps:    testSoftwareDeps,
	}
	ctx = WithCurrentEntity(ctx, ec)

	if softwareDeps, ok := SoftwareDeps(ctx); !ok {
		t.Error("SoftwareDeps failed for context with CurrentEntity")
	} else if !reflect.DeepEqual(softwareDeps, testSoftwareDeps) {
		t.Errorf("SoftwareDeps = %q; want %q", softwareDeps, testSoftwareDeps)
	}
}

func TestServiceDeps(t *testing.T) {
	var testServiceDeps = []string{"foo", "bar"}

	ctx := context.Background()

	if _, ok := ServiceDeps(ctx); ok {
		t.Error("ServiceDeps unexpectedly succeeded for context without CurrentEntity")
	}

	ec := &CurrentEntity{ServiceDeps: testServiceDeps}
	ctx = WithCurrentEntity(ctx, ec)

	if serviceDeps, ok := ServiceDeps(ctx); !ok {
		t.Error("ServiceDeps failed for context with CurrentEntity")
	} else if !reflect.DeepEqual(serviceDeps, testServiceDeps) {
		t.Errorf("ServiceDeps = %q; want %q", serviceDeps, testServiceDeps)
	}
}
