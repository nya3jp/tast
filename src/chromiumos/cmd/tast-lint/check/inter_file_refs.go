// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
)

// InterFileRefs checks that test main files do not have inter-file references.
func InterFileRefs(fs *token.FileSet, f *ast.File) []*Issue {
	const docURL = "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#scoping-and-shared-code"

	filename := fs.Position(f.Package).Filename
	if !isTestMainFile(filename) {
		return nil
	}

	var issues []*Issue
	v := funcVisitor(func(node ast.Node) {
		id, ok := node.(*ast.Ident)
		if !ok || id == nil || id.Obj == nil || id.Obj.Decl == nil {
			return
		}

		decl := id.Obj.Decl.(ast.Node)
		declFn := fs.Position(decl.Pos()).Filename
		if declFn != filename {
			issues = append(issues, &Issue{
				Pos:  fs.Position(node.Pos()),
				Msg:  fmt.Sprintf("Tast forbids inter-file references in test main files; move %s %s in %s to a shared package", id.Obj.Kind, id.Name, filepath.Base(declFn)),
				Link: docURL,
			})
		}
	})
	ast.Walk(v, f)

	return issues
}
