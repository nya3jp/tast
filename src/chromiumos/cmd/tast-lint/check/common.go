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
		!strings.HasSuffix(path, "_test.go") && // exclude unit tests
		filepath.Base(path) != "doc.go" // exclude package documentation
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
