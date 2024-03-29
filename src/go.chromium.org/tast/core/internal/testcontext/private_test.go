// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testcontext_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast/core/internal/testcontext"
)

func TestPrivateData(t *testing.T) {
	ctx := context.Background()

	got, ok := testcontext.PrivateDataFromContext(ctx)
	if ok {
		t.Fatalf("PrivateDataFromContext unexpectedly succeeded for initial Context: %v", got)
	}

	want := testcontext.PrivateData{
		WaitUntilReady:        true,
		WaitUntilReadyTimeout: 10,
	}
	ctx = testcontext.WithPrivateData(ctx, want)

	got, ok = testcontext.PrivateDataFromContext(ctx)
	if !ok {
		t.Error("PrivateDataFromContext failed")
	} else if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("PrivateDataFromContext returned wrong PrivateData (-got +want):\n%s", diff)
	}
}
