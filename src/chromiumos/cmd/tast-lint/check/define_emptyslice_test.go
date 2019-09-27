// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"testing"
)

func TestEmptySlice(t *testing.T) {
	const code = `package main
func main(){
	a := []string{} // error (zero-length but non-nil slice)
	var b []string // valid case (zero-length and nil slice)
	c := []int{} // error (zero-length but non-nil slice)
	d := [2]float{} // valid (not empty)
	e := []bool{true, true, false,} // valid (not empty)
	for _, f := range e {
		if !f {
			g := []newtype.new{} // error (zero-length but non-nil slice)
			h := []*star{} // error (zero-length but non-nil slice)
		}
	}
	i = []string{} // valid (not define-declaration)
	same := [][2]string{} // error (zero-length but non-nil slice)
	lst := []struct { // error (zero-length but non-nil slice)
		ID string
	}{}
	j = []struct { // valid (not define-declaration)
		ID string
	}{}
	k, l := 100, []float{} // error: "l" should be warned
	m, n = "str", []string{} // valid (not define-declaration)
	x, y := someFunction() // valid (not defines empty slice)
}`
	const path = "/src/chromiumos/tast/local/foo.go"
	f, fs := parse(code, path)
	issues := EmptySlice(fs, f)
	expects := []string{
		path + ":3:2: Use 'var' statement when you declare empty slice 'a'",
		path + ":5:2: Use 'var' statement when you declare empty slice 'c'",
		path + ":10:4: Use 'var' statement when you declare empty slice 'g'",
		path + ":11:4: Use 'var' statement when you declare empty slice 'h'",
		path + ":15:2: Use 'var' statement when you declare empty slice 'same'",
		path + ":16:2: Use 'var' statement when you declare empty slice 'lst'",
		path + ":22:5: Use 'var' statement when you declare empty slice 'l'",
	}
	verifyIssues(t, issues, expects)
}
