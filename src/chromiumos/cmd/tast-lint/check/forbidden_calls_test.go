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

	"chromiumos/tast/errors"
)

func main() {
	fmt.Printf("foo")
	fmt.Errorf("foo")
	errors.Errorf("foo")
	time.Sleep(time.Second)
	context.Background()
	context.TODO()
}
`
	expects := []string{
		"testfile.go:12:2: chromiumos/tast/errors.Errorf should be used instead of fmt.Errorf",
		"testfile.go:14:2: time.Sleep ignores context deadline; use testing.Poll or testing.Sleep instead",
		"testfile.go:15:2: context.Background ignores test timeout; use test function's ctx arg instead",
		"testfile.go:16:2: context.TODO ignores test timeout; use test function's ctx arg instead",
	}

	f, fs := parse(code, "testfile.go")
	issues := ForbiddenCalls(fs, f, false)
	verifyIssues(t, issues, expects)
}

func TestAutoFixForbiddenCalls(t *testing.T) {
	files := make(map[string]string)
	expects := make(map[string]string)
	const filename1, filename2, filename3 = "foo.go", "bar.go", "baz.go"
	files[filename1] = `package main

import (
	"fmt"
	"time"

	"chromiumos/tast/errors"
)

func main() {
	fmt.Printf("foo")
	// this is not fixable
	fmt.Errorf("foo")
	errors.Errorf("foo")
	time.Sleep(time.Second)
	context.Background()
	context.TODO()
}
`
	expects[filename1] = `package main

import (
	"fmt"
	"time"

	"chromiumos/tast/errors"
)

func main() {
	fmt.Printf("foo")
	// this is not fixable
	fmt.Errorf("foo")
	errors.Errorf("foo")
	time.Sleep(time.Second)
	context.Background()
	context.TODO()
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
	verifyAutoFix(t, ForbiddenCalls, files, expects)
}
