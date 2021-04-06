// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testcontext

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestPrivateData(t *testing.T) {
	ctx := context.Background()

	got, ok := PrivateDataFromContext(ctx)
	if ok {
		t.Fatalf("PrivateDataFromContext unexpectedly succeeded for initial Context: %v", got)
	}

	want := PrivateData{WaitUntilReady: true}
	ctx = WithPrivateData(ctx, want)

	got, ok = PrivateDataFromContext(ctx)
	if !ok {
		t.Error("PrivateDataFromContext failed")
	} else if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("PrivateDataFromContext returned wrong PrivateData (-got +want):\n%s", diff)
	}
}
