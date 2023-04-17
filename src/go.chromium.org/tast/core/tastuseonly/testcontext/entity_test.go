// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testcontext_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast/core/tastuseonly/testcontext"
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
	const testPrivateAttr = "test_privateAttr"

	ctx := context.Background()

	var panicked bool
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		testcontext.EnsurePrivateAttr(ctx, testPrivateAttr)
	}()
	if !panicked {
		t.Error("EnsurePrivateAttr unexpectedly succeeded for context with no privateAttr")
	}

	ec := &testcontext.CurrentEntity{PrivateAttr: []string{"unrelated_privateAttr"}}
	ctx = testcontext.WithCurrentEntity(ctx, ec)
	panicked = false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		testcontext.EnsurePrivateAttr(ctx, testPrivateAttr)
	}()
	if !panicked {
		t.Error("EnsurePrivateAttr unexpectedly succeeded for context without expected privateAttr")
	}

	ec = &testcontext.CurrentEntity{PrivateAttr: []string{testPrivateAttr}}
	ctx = testcontext.WithCurrentEntity(ctx, ec)
	func() {
		defer func() {
			err := recover()
			if err != nil {
				t.Error("EnsurePrivateAttr unexpectedly failed for context with the expected privateAttr: ", err)
			}
		}()
		testcontext.EnsurePrivateAttr(ctx, testPrivateAttr)
	}()
}

func TestPrivateAttr(t *testing.T) {
	testPrivateAttr := []string{"privateAttr1", "privateAttr2"}
	ec := &testcontext.CurrentEntity{PrivateAttr: testPrivateAttr}
	ctx := testcontext.WithCurrentEntity(context.Background(), ec)
	privateAttr, ok := testcontext.PrivateAttr(ctx)
	if !ok {
		t.Error("PrivateAttr not returned for a context associated with an entity")
	} else if diff := cmp.Diff(privateAttr, testPrivateAttr); diff != "" {
		t.Errorf("PrivateAttr returned unexpected content (-got +want):\n%s", diff)
	}

	// Context not associated with an entity
	ctx = context.Background()
	if privateAttr, ok := testcontext.PrivateAttr(ctx); ok {
		t.Errorf("PrivateAttr returned for a context not associated with entity: %s", privateAttr)
	}
}
