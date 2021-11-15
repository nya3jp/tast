// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"regexp"

	"go.chromium.org/tast/cmd/tast-lint/internal/diff"
)

// commentInImportRegexp matches a comment inside an import block.
var commentInImportRegexp = regexp.MustCompile(`import \([^)]*(//|/\*)`)

// ImportOrder checks if the order of import entries are sorted in the
// following order.
//   - Import entries should be split into three groups; stdlib, third-party
//     packages, and chromiumos packages.
//   - In each group, the entries should be sorted in the lexicographical
//     order.
//   - The groups should be separated by an empty line.
// This order should be same as what "goimports --local chromiumos/" does.
func ImportOrder(path string, in []byte) []*Issue {
	out, err := formatImports(in)
	if err != nil {
		panic(err.Error())
	}

	diff, err := diff.Diff(string(in), string(out))
	if err != nil {
		panic(err.Error())
	}

	if diff != "" {
		return []*Issue{{
			Pos:     token.Position{Filename: path},
			Msg:     fmt.Sprintf("Import should be grouped into standard packages, third-party packages and chromiumos packages in this order separated by empty lines.\nApply the following patch to fix:\n%s", diff),
			Link:    "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#import",
			Fixable: true,
		}}
	}

	// No issue is found.
	return nil
}

type importPos int

const (
	beforeImport importPos = iota
	inImport
	afterImport
)

// trimImportEmptyLine removes empty lines in the import declaration.
func trimImportEmptyLine(in []byte) []byte {
	var lines [][]byte
	current := beforeImport
	for _, line := range bytes.Split(in, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)

		switch current {
		case beforeImport:
			if bytes.Equal(trimmed, []byte("import (")) {
				current = inImport
			}
		case inImport:
			if bytes.Equal(trimmed, []byte(")")) {
				current = afterImport
			}
		}

		if current == inImport && len(trimmed) == 0 {
			// Skip empty line in import section.
			continue
		}
		lines = append(lines, line)
	}
	return bytes.Join(lines, []byte("\n"))
}

// runGoimports runs "goimports --local=chromiumos/". Passed in arg will be
// the stdin for the subprocess. Returns the stdout.
func runGoimports(in []byte) ([]byte, error) {
	_, err := exec.LookPath("goimports")
	if err != nil {
		panic("goimports not found. Please install. If already installed, check that GOPATH[0]/bin is in your PATH.")
	}

	cmd := exec.Command("goimports", "--local=chromiumos/,go.chromium.org/tast")
	cmd.Stdin = bytes.NewBuffer(in)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

func formatImports(in []byte) ([]byte, error) {
	if !goimportApplicable(in) {
		return in, nil
	}

	// goimports preserves import blocks separated by empty lines. To avoid
	// unexpected sorting, remove all empty lines here in import
	// declaration.
	trimmed := trimImportEmptyLine(in)

	// This may potentially raise a false alarm. goimports actually adds
	// or removes some entries in import(), which depends on GOPATH.
	// However, this lint check is running outside of the chroot, unlike
	// actual build, so the GOPATH value and directory structure can be
	// different.
	return runGoimports(trimmed)
}

// ImportOrderAutoFix returns ast.File node whose import was fixed from given node correctly.
func ImportOrderAutoFix(fs *token.FileSet, f *ast.File) (*ast.File, error) {
	// Format ast.File to buffer.
	in, err := formatASTNode(fs, f)
	if err != nil {
		return nil, err
	}
	out, err := formatImports(in)
	if err != nil {
		return nil, err
	}
	// Parse again.
	path := fs.Position(f.Pos()).Filename
	newf, err := parser.ParseFile(fs, path, out, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	return newf, nil
}

// goimportApplicable returns true if there is no comment inside an import block
// since we can't handle it correctly now.
// TODO(crbug.com/900131): Handle it correctly and remove this check.
func goimportApplicable(in []byte) bool {
	return !commentInImportRegexp.Match(in)
}
