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
	a, b, c := []int{}, []int{}, "string" // error: a and b is invalid
	one, two, three, four, five := []int{}, []string{}, []float{}, 100, []byte{}
	if a := []int{}; len(a) == 1 { // this should be ignored
		print(a)
	}
}`
	const path = "/src/chromiumos/tast/local/foo.go"
	f, fs := parse(code, path)
	issues := EmptySlice(fs, f, false)
	expects := []string{
		path + ":3:2: Use 'var' statement when you declare empty slice(s): a",
		path + ":5:2: Use 'var' statement when you declare empty slice(s): c",
		path + ":10:4: Use 'var' statement when you declare empty slice(s): g",
		path + ":11:4: Use 'var' statement when you declare empty slice(s): h",
		path + ":15:2: Use 'var' statement when you declare empty slice(s): same",
		path + ":16:2: Use 'var' statement when you declare empty slice(s): lst",
		path + ":22:2: Use 'var' statement when you declare empty slice(s): l",
		path + ":25:2: Use 'var' statement when you declare empty slice(s): a, b",
		path + ":26:2: Use 'var' statement when you declare empty slice(s): one, two, three, five",
	}
	verifyIssues(t, issues, expects)
}

func TestAutoFixEmptySlice(t *testing.T) {
	const filename = "foo.go"
	files := make(map[string]string)
	files[filename] = `package newpackage

// main do nothing
func main() {
	a := []string{} // error a
	var suji int
	b := []int{} // error b
	x + y
	for _, f := range e {
		if !f {
			g := []newtype.new{} // error g
			var h []*star        // error h
		}
	}
	var same [][2]string // no error same
	lst := []struct {
		ID string
	}{}
	k, l := 0, []string{}                 // comment kl
	a, b, c := []int{}, []int{}, "string" // comment a b c
	var n int                             // comment n
	samesame := [4][]string{}             // comment samesame
	// Comment something.
	someFunction()
	one, two, three, four, five := []int{}, []float{}, []float{}, 100, []byte{} // comment 5
}
`
	expects := make(map[string]string)
	expects[filename] = `package newpackage

// main do nothing
func main() {
	var a []string // error a
	var suji int
	var b []int // error b
	x + y
	for _, f := range e {
		if !f {
			var g []newtype.new // error g
			var h []*star       // error h
		}
	}
	var same [][2]string // no error same
	var lst []struct {
		ID string
	}
	k, l := 0, []string{}                 // comment kl
	a, b, c := []int{}, []int{}, "string" // comment a b c
	var n int                             // comment n
	samesame := [4][]string{}             // comment samesame
	// Comment something.
	someFunction()
	one, two, three, four, five := []int{}, []float{}, []float{}, 100, []byte{} // comment 5
}
`
	verifyAutoFix(t, EmptySlice, files, expects)
}
