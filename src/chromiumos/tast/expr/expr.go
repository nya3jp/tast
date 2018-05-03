// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package expr provides support for evaluating boolean expressions.
package expr

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

// Expr holds a parsed boolean expression that matches some combination of attributes.
//
// Expressions are supplied as strings consisting of the following tokens:
//
//	* Attributes, either as bare identifiers (only if compliant with
//	  https://golang.org/ref/spec#Identifiers) or as double-quoted strings
//	* Binary operators: && (and), || (or)
//	* Unary operator: ! (not)
//	* Grouping: (, )
//
// The expression syntax is conveniently a subset of Go's syntax, so Go's parser and ast
// packages are used to convert the initial expression into a binary expression tree.
//
// After an Expr object is created from a string expression, it can be asked if
// it is satisfied by a supplied set of attributes.
type Expr struct {
	root ast.Expr
}

// exprValidator is used to validate that a parsed Go expression is a
// valid boolean expression. It implements the ast.Visitor interface.
type exprValidator struct {
	err error
}

// setErr stores a formatted error in ev's err field (if not already set) and
// returns nil to instruct ast.Walk to stop walking the expression.
func (ev *exprValidator) setErr(format string, args ...interface{}) ast.Visitor {
	if ev.err == nil {
		ev.err = fmt.Errorf(format, args...)
	}
	return nil
}

// Visit returns itself if n is a valid node in a boolean expression or sets
// ev's err field and returns nil if it isn't.
func (ev *exprValidator) Visit(n ast.Node) ast.Visitor {
	// ast.Walk calls Visit(nil) after visiting non-nil children.
	if n == nil {
		return nil
	}

	switch v := n.(type) {
	case *ast.BinaryExpr:
		if v.Op != token.LAND && v.Op != token.LOR {
			return ev.setErr("invalid binary operator %q", v.Op)
		}
	case *ast.ParenExpr:
	case *ast.UnaryExpr:
		if v.Op != token.NOT {
			return ev.setErr("invalid unary operator %q", v.Op)
		}
	case *ast.Ident:
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return ev.setErr("non-string literal %q", v.Value)
		}
	default:
		return ev.setErr("invalid node of type %T", v)
	}
	return ev
}

// New parses and validates boolean expression s, returning an Expr object
// that can be used to test whether the expression is satisfied by different
// sets of attributes.
func New(s string) (*Expr, error) {
	root, err := parser.ParseExpr(s)
	if err != nil {
		return nil, err
	}

	v := exprValidator{}
	ast.Walk(&v, root)
	return &Expr{root}, v.err
}

// Matches returns true if the expression is satisfied by attributes attr.
func (e *Expr) Matches(attr []string) bool {
	am := make(map[string]struct{})
	for _, a := range attr {
		am[a] = struct{}{}
	}
	return exprTrue(e.root, am)
}

// exprTrue returns true if e is satisfied by attributes attr.
func exprTrue(e ast.Expr, attr map[string]struct{}) bool {
	switch v := e.(type) {
	case *ast.BinaryExpr:
		switch v.Op {
		case token.LAND:
			return exprTrue(v.X, attr) && exprTrue(v.Y, attr)
		case token.LOR:
			return exprTrue(v.X, attr) || exprTrue(v.Y, attr)
		}
	case *ast.ParenExpr:
		return exprTrue(v.X, attr)
	case *ast.UnaryExpr:
		switch v.Op {
		case token.NOT:
			return !exprTrue(v.X, attr)
		}
	case *ast.Ident:
		_, ok := attr[v.Name]
		return ok
	case *ast.BasicLit:
		switch v.Kind {
		case token.STRING:
			// Strip doublequotes.
			_, ok := attr[v.Value[1:len(v.Value)-1]]
			return ok
		}
	}
	return false
}
