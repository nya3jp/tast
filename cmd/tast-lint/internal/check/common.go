// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// entryPathRegexp matches a file name of an entry file.
var entryPathRegexp = regexp.MustCompile(`/src/chromiumos/tast/(?:local|remote)/bundles/[^/]+/[^/]+/[^/]+\.go$`)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// isEntryFile checks if path is an entry file.
func isEntryFile(path string) bool {
	var err error
	exPath := path

	// Unit tests rely on providing fake paths. EvalSymlinks returns an error
	// if the path doesn't exist. To work around this, only evaluate symlinks
	// if the path exists
	if fileExists(exPath) {
		exPath, err = filepath.EvalSymlinks(exPath)
		if err != nil {
			return false
		}
	}

	exPath, err = filepath.Abs(exPath)
	if err != nil {
		return false
	}

	return entryPathRegexp.MatchString(exPath) &&
		!isUnitTestFile(exPath) &&
		filepath.Base(exPath) != "doc.go" // exclude package documentation
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

// stringLitType represent raw string literal or interpreted string literal
// as enumerated value.
type stringLitType int

const (
	rawStringLit stringLitType = iota
	interpretedStringLit
)

// stringLitTypeOf returns the string literal type of s. If s does not belong
// to either raw string literal or interpreted string literal, returns false for ok.
func stringLitTypeOf(s string) (strtype stringLitType, ok bool) {
	if s == "" {
		return 0, false
	}
	quote := s[0]
	if quote != s[len(s)-1] {
		return 0, false
	}
	if quote == '`' {
		return rawStringLit, true
	} else if quote == '"' {
		return interpretedStringLit, true
	}
	return 0, false
}

// quoteAs quotes given unquoted string with double quote or back quote,
// based on stringLiteralType value.
// If specified to be backquoted, but it is impossible, this function
// falls back to the quoting by double-quotes.
func quoteAs(s string, t stringLitType) string {
	if t == rawStringLit && strconv.CanBackquote(s) {
		return "`" + s + "`"
	}
	return strconv.Quote(s)
}

// formatASTNode returns the byte slice of source code from given file nodes.
func formatASTNode(fs *token.FileSet, f *ast.File) ([]byte, error) {
	var buf bytes.Buffer
	if err := format.Node(&buf, fs, f); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
