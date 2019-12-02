// Copyright 2018 The Chromium OS Authors. All rights reserved.
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
	"io/ioutil"
	"os"
	"os/exec"
	"reflect"
	"regexp"

	"chromiumos/tast/diff"
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
	// Skip this file if there is any comment inside an import block since
	// we can't handle it correctly now.
	// TODO(crbug.com/900131): Handle it correctly and remove this check.
	if commentInImportRegexp.Match(in) {
		return nil
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
	out, err := runGoimports(trimmed)
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
		panic("goimports not found. Please install.")
	}

	cmd := exec.Command("goimports", "--local=chromiumos/")
	cmd.Stdin = bytes.NewBuffer(in)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ImportOrderAutoFix automatically fixes import order correctly.
func ImportOrderAutoFix(fs *token.FileSet, f *ast.File, fix bool) []*Issue {
	if !fix {
		return nil
	}
	// Format ast.File to buffer.
	var buf bytes.Buffer
	if err := format.Node(&buf, fs, f); err != nil {
		return nil
	}
	// Trim the buffer data and check with goimports.
	trimmed := trimImportEmptyLine(buf.Bytes())
	out, err := runGoimports(trimmed)
	if err != nil {
		return nil
	}
	// Create a temporary file and write buffer to it.
	tempfile, err := ioutil.TempFile("", "ImportOrder")
	defer os.Remove(tempfile.Name())
	if err != nil {
		return nil
	}
	if err := ioutil.WriteFile(tempfile.Name(), out, 0644); err != nil {
		return nil
	}
	// Parse again.
	newf, err := parser.ParseFile(fs, tempfile.Name(), nil, parser.ParseComments)
	if err != nil {
		return nil
	}
	// Replace value of f as parsed newf.
	reflect.Indirect(reflect.ValueOf(f)).Set(reflect.ValueOf(*newf))
	return nil
}
