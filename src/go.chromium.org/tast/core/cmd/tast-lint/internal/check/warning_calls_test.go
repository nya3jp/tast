// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestWarningCalls(t *testing.T) {
	const code = `package main

import (
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"

	"chromiumos/tast/errors"
)

func main() {
	testing.Sleep()
}
`
	f, fs := parse(code, "testfile.go")
	issues := WarningCalls(fs, f, false)

	if len(issues) != 1 {
		t.Errorf("Warnings should have at least 1 sleep fail")
	}
}

func TestWarningCalls_Exceptions(t *testing.T) {
	const code = `package main

import (
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"

	"chromiumos/tast/errors"
)

func main() {
	testing.Sleep() // GoBigSleepLint: valid testing.sleep
}
`
	var expects []string

	f, fs := parse(code, "src/chromiumos/tast/local/bundles/cros/meta/local_timeout.go")
	issues := WarningCalls(fs, f, false)

	if len(issues) != 0 {
		t.Errorf("Warnings should have 0 sleep warning fail")
	}
}
