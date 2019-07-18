// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"go/token"
	"strings"
)

// Comments checks function comments start with the function name. Golint only checks exported functions, but this examines unexported functions too.
func Comments(fs *token.FileSet, f *ast.File) []*Issue {
	var issues []*Issue

	v := funcVisitor(func(node ast.Node) {
		fn, ok := node.(*ast.FuncDecl)
		if !ok {
			return
		}
		doc := fn.Doc
		if doc == nil {
			return
		}
		prefix := fn.Name.Name + " "
		if strings.HasPrefix(doc.Text(), prefix) {
			return
		}

		addIssue := func(msg, link string) {
			issues = append(issues, &Issue{Pos: fs.Position(doc.Pos()), Msg: msg, Link: link})
		}

		addIssue("Function comments should begin with the function name", "https://github.com/golang/go/wiki/CodeReviewComments#comment-sentences")
	})

	ast.Walk(v, f)
	return issues
}
