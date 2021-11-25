// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package build

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"

	"chromiumos/tast/internal/packages"
)

// sourceCompatVersion describes the compatibility version of the Tast source code.
//
// This number is used to ensure that the source code layout matches what this package expects.
// If one in the source code checkout differs from the one built in the tast executable,
// users should run update_chroot to update the tast executable.
//
// This number must be incremented when a framework change breaks "tast run -build"
// with combination of older tast binary and newer source code.
const sourceCompatVersion = 9

// compatGoPath is the path to this file.
const compatGoPath = "cmd/tast/internal/build/compat.go"

// checkSourceCompat checks if the Tast source code has the same sourceCompatVersion
// as what we know. workspace is the path to the Go workspace containing the Tast
// framework.
func checkSourceCompat(workspace string) error {
	path := filepath.Join(workspace, compatGoPath)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Inspect older and newer location of this file so that compatibility
		// check doesn't fail after we move this file.
		path = filepath.Join(workspace, "src", packages.OldFrameworkPrefix, compatGoPath)
	}
	ver, err := readSourceCompatVersion(path)
	if err != nil {
		return fmt.Errorf("failed to get sourceCompatVersion: %v", err)
	}
	if ver != sourceCompatVersion {
		return fmt.Errorf("sourceCompatVersion is %d, want %d", ver, sourceCompatVersion)
	}
	return nil
}

// readSourceCompatVersion parses the Go file at selfPath and returns the value of
// the top-level sourceCompatVersion constant.
func readSourceCompatVersion(selfPath string) (int, error) {
	f, err := parser.ParseFile(token.NewFileSet(), selfPath, nil, 0)
	if err != nil {
		return 0, err
	}

	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, s := range gd.Specs {
			vs, ok := s.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, id := range vs.Names {
				if id.Name != "sourceCompatVersion" {
					continue
				}
				l, ok := vs.Values[i].(*ast.BasicLit)
				if !ok {
					return 0, errors.New("not a basic literal")
				}
				if l.Kind != token.INT {
					return 0, errors.New("not an integer")
				}
				ver, err := strconv.Atoi(l.Value)
				if err != nil {
					return 0, err
				}
				return ver, nil
			}
		}
	}

	return 0, errors.New("not found")
}
