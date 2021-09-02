// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"path/filepath"
	"strconv"

	"golang.org/x/tools/go/ast/astutil"
)

type deprecatedAPI struct {
	// pkg is the package path containing deprecated API.
	pkg string
	// ident is an exported identifier in the package pkg which is deprecated.
	// If ident is empty, it means the package itself is deprecated.
	// Methods are not supported. Only APIs used in the form of
	// <package name>.<ident> are supported.
	ident string

	alternative string // alternative to use displayed in the error message
	link        string // bug link
}

// DeprecatedAPIs checks if deprecated APIs are used.
func DeprecatedAPIs(fs *token.FileSet, f *ast.File) []*Issue {
	return deprecatedAPIs(fs, f, []*deprecatedAPI{
		{
			pkg:         "chromiumos/tast/local/testexec",
			alternative: "chromiumos/tast/common/testexec",
			link:        "https://crbug.com/1119252",
		},
		{
			pkg:         "golang.org/x/net/context",
			ident:       "",
			alternative: "context",
			link:        "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#Contexts-and-timeouts",
		},
	})
}

func deprecatedAPIs(fs *token.FileSet, f *ast.File, ds []*deprecatedAPI) []*Issue {
	deprecatedPkg := make(map[string]*deprecatedAPI)
	deprecatedPkgIdent := make(map[string]map[string]*deprecatedAPI)
	for _, d := range ds {
		if d.ident == "" {
			deprecatedPkg[d.pkg] = d
			continue
		}
		if _, ok := deprecatedPkgIdent[d.pkg]; !ok {
			deprecatedPkgIdent[d.pkg] = make(map[string]*deprecatedAPI)
		}
		deprecatedPkgIdent[d.pkg][d.ident] = d
	}

	var issues []*Issue

	unquote := func(s string) string {
		res, err := strconv.Unquote(s)
		if err != nil {
			log.Panicf("BUG: %q should be a quoted string", s)
		}
		return res
	}
	// Check deprecated packages.
	for _, i := range f.Imports {
		d, ok := deprecatedPkg[unquote(i.Path.Value)]
		if !ok {
			continue
		}
		issues = append(issues, &Issue{
			Pos:  fs.Position(i.Pos()),
			Msg:  fmt.Sprintf("package %v is deprecated; use %v instead", d.pkg, d.alternative),
			Link: d.link,
		})
	}

	// Check deprecated exported identifiers.
	imports := make(map[string]string) // map package identifier to path
	for _, i := range f.Imports {
		path := unquote(i.Path.Value)
		sel := filepath.Base(path)
		if i.Name != nil {
			sel = i.Name.Name
		}
		imports[sel] = path
	}
	astutil.Apply(f, func(c *astutil.Cursor) bool {
		sel, ok := c.Node().(*ast.SelectorExpr)
		if !ok {
			return true
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		path, ok := imports[x.Name]
		if !ok {
			return true
		}
		m, ok := deprecatedPkgIdent[path]
		if !ok {
			return true
		}
		d, ok := m[sel.Sel.Name]
		if !ok {
			return true
		}

		issues = append(issues, &Issue{
			Pos:  fs.Position(x.Pos()),
			Msg:  fmt.Sprintf("%v.%v is deprecated; use %v instead", d.pkg, d.ident, d.alternative),
			Link: d.link,
		})
		return true
	}, nil)

	return issues
}
