// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/ast"
	"path/filepath"
	"regexp"
	"strings"
)

// testMainPathRegexp matches a file name of a Tast test main file.
var testMainPathRegexp = regexp.MustCompile(`/src/chromiumos/tast/(?:local|remote)/bundles/[^/]+/[^/]+/[^/]+\.go$`)

// isTestMainFile checks if path is a Test test main file.
func isTestMainFile(path string) bool {
	path, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return testMainPathRegexp.MatchString(path) &&
		!isUnitTestFile(path) &&
		filepath.Base(path) != "doc.go" // exclude package documentation
}

// isUnitTestFile returns true if the supplied path corresponds to a unit test file.
func isUnitTestFile(path string) bool {
	return strings.HasSuffix(path, "_test.go")
}

// funcVisitor is an implementation of ast.Visitor to scan all nodes.
type funcVisitor func(node ast.Node)

func (v funcVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}
	v(node)
	return v
}

// toQualifiedName stringifies the given node, which is either
// - an ast.Ident node
// - an ast.SelectorExpr node whose .X node is convertible by toQualifiedName.
// If failed, returns an empty string.
func toQualifiedName(node ast.Node) string {
	var comp []string
	for {
		s, ok := node.(*ast.SelectorExpr)
		if !ok {
			break
		}
		comp = append(comp, s.Sel.Name)
		node = s.X
	}

	id, ok := node.(*ast.Ident)
	if !ok {
		return ""
	}
	comp = append(comp, id.Name)

	// Reverse the comp, then join with '.'.
	for i, j := 0, len(comp)-1; i < j; i, j = i+1, j-1 {
		comp[i], comp[j] = comp[j], comp[i]
	}
	return strings.Join(comp, ".")
}
