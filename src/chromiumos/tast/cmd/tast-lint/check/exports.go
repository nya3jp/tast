// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
)

// decl is a small representation of a declaration in Go code.
type decl struct {
	token token.Token
	name  *ast.Ident
	decl  ast.Decl
}

// getDecls returns a list of declarations in a file.
func getDecls(f *ast.File) []decl {
	var decls []decl
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			decls = append(decls, decl{token.FUNC, d.Name, d})
		case *ast.GenDecl:
			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.TypeSpec:
					decls = append(decls, decl{d.Tok, s.Name, d})
				case *ast.ValueSpec:
					for _, name := range s.Names {
						decls = append(decls, decl{d.Tok, name, d})
					}
				}
			}
		}
	}
	return decls
}

// Exports checks that an entry file exports exactly one symbol, either a test
// main function or a gRPC service implementation type.
func Exports(fs *token.FileSet, f *ast.File) []*Issue {
	const docURL = "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#scoping-and-shared-code"

	filename := fs.Position(f.Package).Filename
	if !isEntryFile(filename) {
		return nil
	}

	var issues []*Issue

	var expDecls []decl
	for _, d := range getDecls(f) {
		if d.name.IsExported() {
			if d.token == token.FUNC && d.decl.(*ast.FuncDecl).Recv != nil {
				continue
			}
			if d.token == token.FUNC || d.token == token.TYPE {
				expDecls = append(expDecls, d)
				continue
			}
			issues = append(issues, &Issue{
				Pos:  fs.Position(d.name.NamePos),
				Msg:  fmt.Sprintf("Tast requires exactly one symbol (test function or service type) to be exported in an entry file; unexport %s %s", d.token, d.name.Name),
				Link: docURL,
			})
		}
	}

	if len(expDecls) == 0 {
		issues = append(issues, &Issue{
			Pos:  token.Position{Filename: filename},
			Msg:  "Tast requires exactly one symbol (test function or service type) to be exported in an entry file",
			Link: docURL,
		})
	} else if len(expDecls) >= 2 {
		for _, d := range expDecls {
			issues = append(issues, &Issue{
				Pos:  fs.Position(d.name.NamePos),
				Msg:  fmt.Sprintf("Tast requires exactly one symbol (test function or service type) to be exported in an entry file; unexport %s %s if it is not one", d.token, d.name.Name),
				Link: docURL,
			})
		}
	}

	// TODO(nya): Check the test function name.
	return issues
}
