// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testcontext_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"

	"chromiumos/tast/internal/testcontext"
)

func TestOutDir(t *testing.T) {
	const testOutDir = "/mock/outdir"

	ctx := context.Background()

	if _, ok := testcontext.OutDir(ctx); ok {
		t.Error("OutDir unexpectedly succeeded for context without CurrentEntity")
	}

	ec := &testcontext.CurrentEntity{OutDir: testOutDir}
	ctx = testcontext.WithCurrentEntity(ctx, ec)

	if outDir, ok := testcontext.OutDir(ctx); !ok {
		t.Error("OutDir failed for context with CurrentEntity")
	} else if outDir != testOutDir {
		t.Errorf("OutDir = %q; want %q", outDir, testOutDir)
	}

	ec = &testcontext.CurrentEntity{OutDir: ""}
	ctx = testcontext.WithCurrentEntity(ctx, ec)

	if _, ok := testcontext.OutDir(ctx); ok {
		t.Error("OutDir unexpectedly succeeded for empty OutDir")
	}
}

func TestSoftwareDeps(t *testing.T) {
	var testSoftwareDeps = []string{"foo", "bar"}

	ctx := context.Background()

	if _, ok := testcontext.SoftwareDeps(ctx); ok {
		t.Error("SoftwareDeps unexpectedly succeeded for context without CurrentEntity")
	}

	if _, ok := testcontext.SoftwareDeps(testcontext.WithCurrentEntity(ctx, &testcontext.CurrentEntity{})); ok {
		t.Error("SoftwareDeps unexpectedly succeeded for context without SoftwareDeps")
	}

	ec := &testcontext.CurrentEntity{
		HasSoftwareDeps: true,
		SoftwareDeps:    testSoftwareDeps,
	}
	ctx = testcontext.WithCurrentEntity(ctx, ec)

	if softwareDeps, ok := testcontext.SoftwareDeps(ctx); !ok {
		t.Error("SoftwareDeps failed for context with CurrentEntity")
	} else if !reflect.DeepEqual(softwareDeps, testSoftwareDeps) {
		t.Errorf("SoftwareDeps = %q; want %q", softwareDeps, testSoftwareDeps)
	}
}

func TestServiceDeps(t *testing.T) {
	var testServiceDeps = []string{"foo", "bar"}

	ctx := context.Background()

	if _, ok := testcontext.ServiceDeps(ctx); ok {
		t.Error("ServiceDeps unexpectedly succeeded for context without CurrentEntity")
	}

	ec := &testcontext.CurrentEntity{ServiceDeps: testServiceDeps}
	ctx = testcontext.WithCurrentEntity(ctx, ec)

	if serviceDeps, ok := testcontext.ServiceDeps(ctx); !ok {
		t.Error("ServiceDeps failed for context with CurrentEntity")
	} else if !reflect.DeepEqual(serviceDeps, testServiceDeps) {
		t.Errorf("ServiceDeps = %q; want %q", serviceDeps, testServiceDeps)
	}
}

func TestEnsureLable(t *testing.T) {
	const testLabel = "test_label"

	ctx := context.Background()

	var panicked bool
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		testcontext.EnsureLabel(ctx, testLabel)
	}()
	if !panicked {
		t.Error("EnsureLabel unexpectedly succeeded for context with no labels")
	}

	ec := &testcontext.CurrentEntity{Labels: []string{"unrelated_label"}}
	ctx = testcontext.WithCurrentEntity(ctx, ec)
	panicked = false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		testcontext.EnsureLabel(ctx, testLabel)
	}()
	if !panicked {
		t.Error("EnsureLabel unexpectedly succeeded for context without expected label")
	}

	ec = &testcontext.CurrentEntity{Labels: []string{testLabel}}
	ctx = testcontext.WithCurrentEntity(ctx, ec)
	func() {
		defer func() {
			err := recover()
			if err != nil {
				t.Error("EnsureLabel unexpectedly failed for context with the expected label: ", err)
			}
		}()
		testcontext.EnsureLabel(ctx, testLabel)
	}()
}

func TestLabels(t *testing.T) {
	testLabels := []string{"label1", "label2"}
	ec := &testcontext.CurrentEntity{Labels: testLabels}
	ctx := testcontext.WithCurrentEntity(context.Background(), ec)
	labels, ok := testcontext.Labels(ctx)
	if !ok {
		t.Error("Labels not returned for a context associated with an entity")
	} else if diff := cmp.Diff(labels, testLabels); diff != "" {
		t.Errorf("Labels returned unexpected content (-got +want):\n%s", diff)
	}

	// Context not associated with an entity
	ctx = context.Background()
	if labels, ok := testcontext.Labels(ctx); ok {
		t.Errorf("Labels returned for a context not associated with entity: %s", labels)
	}
}
