// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

const funcParamsErrMsg = "When two or more consecutive named function parameters share a type, you can omit the type from all but the last"

func TestFuncParams(t *testing.T) {
	const code = `package main

type f func(int, int)

func g(a, b int) (c int, d int) {
	_ = func(a int, b int) {}
	_ = func(a int, b ...int) {}
}

func (*x) h(a int, b, c string, d string, e int, f string) (int, int) {
	_ = func() (a *x, b *x) {}
	_ = func() (a x, b *x) {}
}
`
	const path = "/src/chromiumos/tast/local/foo.go"
	f, fs := parse(code, path)
	issues := FuncParams(fs, f, false)
	expects := []string{
		path + ":5:21: " + funcParamsErrMsg,
		path + ":6:13: " + funcParamsErrMsg,
		path + ":10:25: " + funcParamsErrMsg,
		path + ":11:16: " + funcParamsErrMsg,
	}
	verifyIssues(t, issues, expects)
}
func TestFuncParamsFix(t *testing.T) {
	const code = `package main

type f func(int, int)

func g(a, b int) (c int, d int) {
	_ = func(a int, b int) {}
	_ = func(a int, b ...int) {}
}

func (*x) h(a int, b, c string, d string, e int, f string) (int, int) {
	_ = func() (a *x, b *x) {}
	_ = func() (a x, b *x) {}
	_ = func() (/*a*/ a x, b x) {} // test comments are not lost
	_ = func() (a /*a*/ x, b x) {}
	_ = func() (a x /*a*/, b x) {}
	_ = func() (a x, /*b*/ b x) {}
	_ = func() (a x, b /*b*/ x) {}
	_ = func() (a x, b x /*b*/) {}
	_ = func() ( // begin
		a x, // a
		// b
		b x,
		// c
		c x, // x
		// end
		) {}
}
`
	const want = `package main

type f func(int, int)

func g(a, b int) (c, d int) {
	_ = func(a, b int) {}
	_ = func(a int, b ...int) {}
}

func (*x) h(a int, b, c, d string, e int, f string) (int, int) {
	_ = func() (a, b *x) {}
	_ = func() (a x, b *x) {}
	_ = func() ( /*a*/ a, b x) {} // test comments are not lost
	_ = func() (a /*a*/, b x) {}
	_ = func() (a /*a*/, b x) {}
	_ = func() (a /*b*/, b x) {}
	_ = func() (a, b /*b*/ x) {}
	_ = func() (a, b x /*b*/) {}
	_ = func() ( // begin
		a, // a
		// b
		b,
		// c
		c x, // x
		// end
	) {
	}
}
`
	const path = "foo.go"
	verifyAutoFix(t, FuncParams, map[string]string{path: code}, map[string]string{path: want})
}
