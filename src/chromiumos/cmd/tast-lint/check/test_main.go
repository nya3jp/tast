// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"unicode"
	"unicode/utf8"
)

// TestMain checks no top-level declarations appear in test main files.
func TestMain(fs *token.FileSet, f *ast.File) []*Issue {
	filename := fs.Position(f.Package).Filename
	if !isTestMainFile(filename) {
		return nil
	}

	type badDecl struct {
		token token.Token
		name  *ast.Ident
	}

	var tests []*ast.Ident
	var bads []badDecl
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if c, _ := utf8.DecodeRuneInString(d.Name.Name); unicode.IsUpper(c) {
				// All exported functions are considered as test functions.
				tests = append(tests, d.Name)
			} else if d.Name.Name != "init" {
				// Non-exported functions except init are all bad.
				bads = append(bads, badDecl{token.FUNC, d.Name})
			}
		case *ast.GenDecl:
			// All non-function declarations are bad.
			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.TypeSpec:
					bads = append(bads, badDecl{d.Tok, s.Name})
				case *ast.ValueSpec:
					for _, name := range s.Names {
						bads = append(bads, badDecl{d.Tok, name})
					}
				}
			}
		}
	}

	var issues []*Issue

	// A test main file should have exactly one test function.
	if len(tests) == 0 {
		issues = append(issues, &Issue{
			Pos: token.Position{Filename: filename},
			Msg: "Tast mandates exactly one exported test function to be declared in a test main file",
		})
	} else if len(tests) >= 2 {
		for _, name := range tests {
			issues = append(issues, &Issue{
				Pos: fs.Position(name.NamePos),
				Msg: "Tast mandates exactly one exported test function to be declared in a test main file",
			})
		}
	}

	for _, bad := range bads {
		issues = append(issues, &Issue{
			Pos: fs.Position(bad.name.NamePos),
			Msg: fmt.Sprintf("Tast forbids %s %s to be declared at top level; move it to the test function or subpackages", bad.token, bad.name.Name),
		})
	}

	return issues
}
