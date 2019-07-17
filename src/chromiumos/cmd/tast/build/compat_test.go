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

func checkSourceCompatWithCode(t *testing.T, code string) error {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	if err := testutil.WriteFiles(td, map[string]string{
		compatGoPath: code,
	}); err != nil {
		t.Fatal("WriteFiles failed: ", err)
	}

	return checkSourceCompat(td)
}

func TestCheckSourceCompat(t *testing.T) {
	code := fmt.Sprintf(`
package build
const sourceCompatVersion = %d
`, sourceCompatVersion)
	if err := checkSourceCompatWithCode(t, code); err != nil {
		t.Error("checkSourceCompat failed: ", err)
	}
}

func TestCheckSourceCompatCompound(t *testing.T) {
	code := fmt.Sprintf(`
package build
const (
  foo = "foo"
  sourceCompatVersion = %d
  bar = struct{}{}
)
`, sourceCompatVersion)
	if err := checkSourceCompatWithCode(t, code); err != nil {
		t.Error("checkSourceCompat failed: ", err)
	}
}

func TestCheckSourceCompatFail(t *testing.T) {
	const code = `
package build
const sourceCompatVersion = -28
`
	if err := checkSourceCompatWithCode(t, code); err == nil {
		t.Error("checkSourceCompat unexpectedly succeeded")
	}
}

func TestCheckSourceCompatDeclMissing(t *testing.T) {
	code := fmt.Sprintf(`
package build
const fooVersion = %d
`, sourceCompatVersion)
	if err := checkSourceCompatWithCode(t, code); err == nil {
		t.Error("checkSourceCompat unexpectedly succeeded")
	}
}

func TestCheckSourceCompatWrongType(t *testing.T) {
	const code = `
package build
const sourceCompatVersion = "hi"
`
	if err := checkSourceCompatWithCode(t, code); err == nil {
		t.Error("checkSourceCompat unexpectedly succeeded")
	}
}

func TestCheckSourceCompatFileMissing(t *testing.T) {
	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	if err := checkSourceCompat(td); err == nil {
		t.Error("checkSourceCompat unexpectedly succeeded")
	}
}
