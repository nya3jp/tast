// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/token"
	"testing"
)

func TestImportOrderGood(t *testing.T) {
	const code = `package main

import (
	"fmt"

	"github.com/godbus/dbus"

	"chromiumos/tast/errors"
	"go.chromium.org/tast/errors"
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
	const (
		code = `package main

import (
	"fmt"
	"github.com/godbus/dbus"
	"chromiumos/tast/errors"
	"go.chromium.org/tast/errors"
)

func Foo() {
	fmt.Print("")
	dbus.New()
	errors.New()
}
`
		diff = `@@ -2,7 +2,9 @@
 
 import (
 	"fmt"
+
 	"github.com/godbus/dbus"
+
 	"chromiumos/tast/errors"
 	"go.chromium.org/tast/errors"
 )
`
	)

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

func TestImportOrderCommentInImportBlock(t *testing.T) {
	const code = `package main

import (
	"fmt"

	// some comment
	"chromiumos/tast/errors"
)

func Foo() {
	fmt.Println(errors.New("foo"))
}
`

	// The import order is good, so no issue.
	var expects []string
	issues := ImportOrder("testfile.go", []byte(code))
	verifyIssues(t, issues, expects)
}

func TestAutoFixImportOrder(t *testing.T) {
	const filename1, filename2 = "testfile1.go", "testfile2.go"
	files := make(map[string]string)
	files[filename1] = `// Package main
package main

import (
	"fmt"
	"github.com/godbus/dbus"
	"chromiumos/tast/errors"
)

// Foo does foo
func Foo() {
	fmt.Print("")
	dbus.New()
	errors.New()
}
`
	files[filename2] = `// Package main
package main

import (
	"fmt"

	"chromiumos/tast/errors"

	"github.com/godbus/dbus"
)

// Foo does foo
func Foo() {
	fmt.Print("")
	dbus.New()
	errors.New()
}
`
	expects := make(map[string]string)
	expects[filename1] = `// Package main
package main

import (
	"fmt"

	"github.com/godbus/dbus"

	"chromiumos/tast/errors"
)

// Foo does foo
func Foo() {
	fmt.Print("")
	dbus.New()
	errors.New()
}
`
	expects[filename2] = `// Package main
package main

import (
	"fmt"

	"github.com/godbus/dbus"

	"chromiumos/tast/errors"
)

// Foo does foo
func Foo() {
	fmt.Print("")
	dbus.New()
	errors.New()
}
`
	verifyAutoFix(t, importOrderWrapper, files, expects)
}

func importOrderWrapper(fs *token.FileSet, f *ast.File, fix bool) []*Issue {
	newf, err := ImportOrderAutoFix(fs, f)
	if err != nil {
		return nil
	}
	*f = *newf
	return nil
}
