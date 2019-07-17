// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package build

import (
	"fmt"
	"os"
	"testing"

	"chromiumos/tast/testutil"
)

func TestCheckSourceCompat(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	code := fmt.Sprintf("package build\nconst sourceCompatVersion = %d", sourceCompatVersion)
	if err := testutil.WriteFiles(td, map[string]string{
		compatGoPath: code,
	}); err != nil {
		t.Fatal("WriteFiles failed: ", err)
	}

	if err := checkSourceCompat(td); err != nil {
		t.Error("checkSourceCompat failed: ", err)
	}
}

func TestCheckSourceCompatFail(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	const code = "package build\nconst sourceCompatVersion = -28"
	if err := testutil.WriteFiles(td, map[string]string{
		compatGoPath: code,
	}); err != nil {
		t.Fatal("WriteFiles failed: ", err)
	}

	if err := checkSourceCompat(td); err == nil {
		t.Error("checkSourceCompat unexpectedly succeeded")
	}
}

func TestCheckSourceCompatMissing(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	if err := checkSourceCompat(td); err == nil {
		t.Error("checkSourceCompat unexpectedly succeeded")
	}
}
