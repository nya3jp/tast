// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lint

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"

	"go.chromium.org/tast/cmd/tast-lint/internal/git"
)

// cachedParser is a Go parser with cache.
// This parser resolves references within a package, which is useful to inspect
// inter-file dependencies within a package.
// On the other hand, it does not resolve imports. This is by design because,
// when tast-lint is run, full GOPATH is usually not available.
type cachedParser struct {
	fs   *token.FileSet
	git  *git.Git
	pkgs map[string]*ast.Package
}

// newCachedParser creates a cachedParser.
func newCachedParser(g *git.Git) *cachedParser {
	return &cachedParser{
		fs:   token.NewFileSet(),
		git:  g,
		pkgs: make(map[string]*ast.Package),
	}
}

// parseFile parses a Go code and returns its AST.
// This function also parses other Go files in the same directory and resolves
// references within the package (but does not resolve imports).
func (p *cachedParser) parseFile(path string) (*ast.File, error) {
	pkg, err := p.parsePackage(filepath.Dir(path))
	if err != nil {
		return nil, err
	}

	f := pkg.Files[path]
	if f == nil {
		return nil, fmt.Errorf("%s not found", path)
	}
	return f, nil
}

// parsePackage parses a Go package located in dir.
// This function resolves references within the package, but does not resolve
// imports.
func (p *cachedParser) parsePackage(dir string) (*ast.Package, error) {
	if pkg := p.pkgs[dir]; pkg != nil {
		return pkg, nil
	}

	fns, err := p.git.ListDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to list files at %s: %v", dir, err)
	}

	files := map[string]*ast.File{}
	for _, fn := range fns {
		if !strings.HasSuffix(fn, ".go") {
			continue
		}

		path := filepath.Join(dir, fn)
		f, err := p.parseFileAlone(path)
		if err != nil {
			return nil, err
		}
		files[path] = f
	}

	// Ignore errors on creating packages because we don't resolve imports so
	// there should still be unresolved references.
	pkg, _ := ast.NewPackage(p.fs, files, nil, nil)

	p.pkgs[dir] = pkg
	return pkg, nil
}

// parseFileAlone parses a Go code and returns its AST.
// External references are not resolved.
func (p *cachedParser) parseFileAlone(path string) (*ast.File, error) {
	code, err := p.git.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %v", path, err)
	}

	f, err := parser.ParseFile(p.fs, path, code, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %v", path, err)
	}
	return f, nil
}
