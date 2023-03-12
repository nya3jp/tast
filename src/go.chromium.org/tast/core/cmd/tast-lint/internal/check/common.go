// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
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

// policyNames retrieves the policy names from the defs.go definition file. It
// returns a map where they key represent the name of the policy, and the value
// it a bool which is true for those policies that have the Name function
// defined, false otherwise.
func policyNames() map[string]bool {
	// Get the path to the common file.
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("Could not get caller information")
	}

	src := strings.Split(filename, "platform")[0]
	// Build defs path.
	defs := path.Join(src, "platform/tast-tests/src/chromiumos/tast/common/policy/defs.go")
	if !fileExists(defs) {
		panic(fmt.Sprintf("Path %s does not exist", defs))
	}

	fs := token.NewFileSet()
	file, err := parser.ParseFile(fs, defs, nil, parser.ParseComments)
	if err != nil {
		panic(err)
	}

	m := make(map[string]bool)

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		// Check if declaration is a type definition.
		if ok && genDecl.Tok == token.TYPE && len(genDecl.Specs) == 1 {
			typeSpec, ok := genDecl.Specs[0].(*ast.TypeSpec)
			if ok {
				m[typeSpec.Name.Name] = false
			}

			continue
		}

		funcDecl, ok := decl.(*ast.FuncDecl)
		// Check if the declaration is the Name function definition implemented for
		// each policy.
		if ok && funcDecl.Name.Name == "Name" && len(funcDecl.Body.List) == 1 {
			returnStmt, ok := funcDecl.Body.List[0].(*ast.ReturnStmt)
			if ok && len(returnStmt.Results) == 1 {
				basicLit, ok := returnStmt.Results[0].(*ast.BasicLit)
				if ok && basicLit.Kind == token.STRING {
					policyName := basicLit.Value[1 : len(basicLit.Value)-1]
					_, ok = m[policyName]
					if ok {
						m[policyName] = true
					}
				}
			}
		}
	}

	return m
}

// union adds all key-value pairs from maps a and b. If a and b have an equal key
// then the value from a will be kept.
func union[K comparable, V any](a, b map[K]V) map[K]V {
	c := make(map[K]V)
	for k, v := range a {
		c[k] = v
	}

	for k, v := range b {
		_, present := c[k]
		if !present {
			c[k] = v
		}
	}

	return c
}
