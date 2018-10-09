// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import "testing"

func TestImportOrderGood(t *testing.T) {
	const code = `package main

import (
	"fmt"

	"github.com/godbus/dbus"

	"chromiumos/tast/errors"
)

func Foo() {
	fmt.Print("")
	dbus.New()
	errors.New()
}
`

	// The import order is good, so no issue.
	var expects []string
	issues := ImportOrder("testfile.go", []byte(code))
	verifyIssues(t, issues, expects)
}

func TestImportOrderGroup(t *testing.T) {
	const code = `package main

import (
	"fmt"
	"github.com/godbus/dbus"
	"chromiumos/tast/errors"
)

func Foo() {
	fmt.Print("")
	dbus.New()
	errors.New()
}
`

	const diff = `@@ -2,7 +2,9 @@
 
 import (
 	"fmt"
+
 	"github.com/godbus/dbus"
+
 	"chromiumos/tast/errors"
 )
 
`

	expects := []string{
		"testfile.go: Import should be grouped into standard packages, third-party packages and chromiumos packages in this order separated by empty lines.\nApply the following patch to fix:\n" + diff,
	}
	issues := ImportOrder("testfile.go", []byte(code))
	verifyIssues(t, issues, expects)
}

func TestImportOrderGroupOrder(t *testing.T) {
	const code = `package main

import (
	"fmt"

	"chromiumos/tast/errors"

	"github.com/godbus/dbus"
)

func Foo() {
	fmt.Print("")
	dbus.New()
	errors.New()
}
`

	const diff = `@@ -3,9 +3,9 @@
 import (
 	"fmt"
 
-	"chromiumos/tast/errors"
-
 	"github.com/godbus/dbus"
+
+	"chromiumos/tast/errors"
 )
 
 func Foo() {
`

	expects := []string{
		"testfile.go: Import should be grouped into standard packages, third-party packages and chromiumos packages in this order separated by empty lines.\nApply the following patch to fix:\n" + diff,
	}
	issues := ImportOrder("testfile.go", []byte(code))
	verifyIssues(t, issues, expects)
}
