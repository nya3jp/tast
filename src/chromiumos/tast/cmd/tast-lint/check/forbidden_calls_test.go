// Copyright 2018 The Chromium OS Authors. All rights reserved.
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

	"github.com/godbus/dbus"

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
}
`
	expects := []string{
		"testfile.go:14:2: chromiumos/tast/errors.Errorf should be used instead of fmt.Errorf",
		"testfile.go:16:2: time.Sleep ignores context deadline; use testing.Poll or testing.Sleep instead",
		"testfile.go:17:2: context.Background ignores test timeout; use test function's ctx arg instead",
		"testfile.go:18:2: context.TODO ignores test timeout; use test function's ctx arg instead",
		"testfile.go:19:2: dbus.SystemBus may reorder signals; use chromiumos/tast/local/dbusutil.SystemBus instead",
		"testfile.go:20:2: dbus.SystemBusPrivate may reorder signals; use chromiumos/tast/local/dbusutil.SystemBusPrivate instead",
	}

	f, fs := parse(code, "testfile.go")
	issues := ForbiddenCalls(fs, f, false)
	verifyIssues(t, issues, expects)
}

func TestAutoFixForbiddenCalls(t *testing.T) {
	files := make(map[string]string)
	expects := make(map[string]string)
	const filename1, filename2, filename3, filename4 = "foo.go", "bar.go", "baz.go", "dbus.go"

	// fmt.Errorf -> chromiumos/tast/errors.Errorf cannot be
	// automatically fixed in this file right now because it contains an
	// "errors" identifier node.
	files[filename1] = `package main

import (
	"fmt"
	"time"

	"github.com/godbus/dbus"

	"chromiumos/tast/errors"
)

func main() {
	fmt.Printf("foo")
	// This is not fixable
	fmt.Errorf("foo")
	
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
	// This is not fixable
	fmt.Errorf("foo")

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
	"github.com/godbus/dbus"
)

func main() {
	dbus.SystemBus()
	dbus.SystemBusPrivate(dbus.WithHandler(nil))
}
`

	expects[filename4] = `package main

import (
	"github.com/godbus/dbus"

	"chromiumos/tast/local/dbusutil"
)

func main() {
	dbusutil.SystemBus()
	dbusutil.SystemBusPrivate(dbus.WithHandler(nil))
}
`

	verifyAutoFix(t, ForbiddenCalls, files, expects)
}
