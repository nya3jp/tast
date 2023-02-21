// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestForbiddenCalls(t *testing.T) {
	const code = `package main

import (
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"

	"chromiumos/tast/errors"
)

func main() {
	fmt.Printf("foo")
	fmt.Errorf("foo")
	errors.Errorf("foo")
	time.Sleep(time.Second)
	context.Background()
	context.TODO()
	dbus.SystemBus()
	dbus.SystemBusPrivate()
	os.Chdir("tmp")
}
`
	expects := []string{
		"testfile.go:14:2: chromiumos/tast/errors.Errorf should be used instead of fmt.Errorf",
		"testfile.go:16:2: time.Sleep ignores context deadline; use testing.Poll or testing.Sleep instead",
		"testfile.go:17:2: context.Background ignores test timeout; use test function's ctx arg instead",
		"testfile.go:18:2: context.TODO ignores test timeout; use test function's ctx arg instead",
		"testfile.go:19:2: dbus.SystemBus may reorder signals; use chromiumos/tast/local/dbusutil.SystemBus instead",
		"testfile.go:20:2: dbus.SystemBusPrivate may reorder signals; use chromiumos/tast/local/dbusutil.SystemBusPrivate instead",
		"testfile.go:21:2: os.Chdir changes the shared CWD of the process, reference files using absolute paths.",
	}

	f, fs := parse(code, "testfile.go")
	issues := ForbiddenCalls(fs, f, false)
	verifyIssues(t, issues, expects)
}

func TestForbiddenCalls_Exceptions(t *testing.T) {
	const code = `package main

import (
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"

	"chromiumos/tast/errors"
)

func main() {
	fmt.Printf("foo")
	fmt.Errorf("foo")
	errors.Errorf("foo")
	time.Sleep(time.Second)
	context.Background()
	context.TODO()
	dbus.SystemBus()
	dbus.SystemBusPrivate()
	os.Chdir("tmp")
}
`
	var expects []string

	f, fs := parse(code, "src/chromiumos/tast/local/bundles/cros/meta/local_timeout.go")
	issues := ForbiddenCalls(fs, f, false)
	verifyIssues(t, issues, expects)

	f, fs = parse(code, "src/chromiumos/tast/remote/bundles/cros/meta/remote_timeout.go")
	issues = ForbiddenCalls(fs, f, false)
	verifyIssues(t, issues, expects)
}

func TestAutoFixForbiddenCalls(t *testing.T) {
	files := make(map[string]string)
	expects := make(map[string]string)
	const filename1, filename2, filename3, filename4, filename5, filename6, filename7 = "foo.go", "bar.go", "baz.go",
		"dbus.go", "foo1.go", "bar1.go", "baz1.go"

	files[filename1] = `package main

import (
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"

	"chromiumos/tast/errors"
)

func main() {
	fmt.Printf("foo")
	fmt.Errorf("foo") // This is fixable

	errors.Errorf("foo")
	time.Sleep(time.Second)
	context.Background()
	context.TODO()
	dbus.SystemBus()
	dbus.SystemBusPrivate()
}
`

	expects[filename1] = `package main

import (
	"context"
	"fmt"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/dbusutil"
)

func main() {
	fmt.Printf("foo")
	errors.Errorf("foo") // This is fixable

	errors.Errorf("foo")
	time.Sleep(time.Second)
	context.Background()
	context.TODO()
	dbusutil.SystemBus()
	dbusutil.SystemBusPrivate()
}
`
	files[filename2] = `package main

import (
	"fmt"
)

func main() {
	fmt.Errorf("foo")
	fmt.Println("bar")
}
`
	expects[filename2] = `package main

import (
	"fmt"

	"chromiumos/tast/errors"
)

func main() {
	errors.Errorf("foo")
	fmt.Println("bar")
}
`
	files[filename3] = `package main

import (
	"fmt"
)

func main() {
	fmt.Errorf("foo")
}
`
	expects[filename3] = `package main

import (
	"chromiumos/tast/errors"
)

func main() {
	errors.Errorf("foo")
}
`
	files[filename4] = `package main

import (
	"github.com/godbus/dbus/v5"
)

func main() {
	dbus.SystemBus()
	dbus.SystemBusPrivate(dbus.WithHandler(nil))
}
`

	expects[filename4] = `package main

import (
	"github.com/godbus/dbus/v5"

	"chromiumos/tast/local/dbusutil"
)

func main() {
	dbusutil.SystemBus()
	dbusutil.SystemBusPrivate(dbus.WithHandler(nil))
}
`
	files[filename5] = `package main

import (
	. "chromiumos/tast/errors"

	"fmt"
)

func main() {
	New("import chromiumos/tast/errors with alias")
	fmt.Errorf("This is not fixable")
}`
	expects[filename5] = `package main

import (
	. "chromiumos/tast/errors"

	"fmt"
)

func main() {
	New("import chromiumos/tast/errors with alias")
	fmt.Errorf("This is not fixable")
}
`
	files[filename6] = `package main

import (
	"fmt"

	"chromiumos/tast/errors"
)

func foo() error {
	return errors.New("foo")
}

func bar(n int) error {
	return fmt.Errorf("%d", n) // fixable, uses existing import
}`
	expects[filename6] = `package main

import (
	"chromiumos/tast/errors"
)

func foo() error {
	return errors.New("foo")
}

func bar(n int) error {
	return errors.Errorf("%d", n) // fixable, uses existing import
}`
	files[filename7] = `package main

import "fmt"

func errors() bool {
	return false
}
func main() {
	fmt.Errorf("error") // Not fixable
	errors()
}`
	expects[filename7] = `package main

import "fmt"

func errors() bool {
	return false
}
func main() {
	fmt.Errorf("error") // Not fixable
	errors()
}`

	verifyAutoFix(t, ForbiddenCalls, files, expects)
}
