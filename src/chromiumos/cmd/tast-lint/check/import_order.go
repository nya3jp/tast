// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"bytes"
	"go/token"
	"os/exec"
)

// ImportOrder checks if the order of import entries are sorted in the
// following order.
// - Import entries should be split into three groups; stdlib, third_party lib,
//   and chromiumos libs.
// - In each group, the entries should be sorted in the lexicographical order.
// - The groups should be separated by an empty line.
// This order should be same as what "goimports --local chromiumos/" does.
func ImportOrder(path string, in []byte) []*Issue {
	// goimports preserves import blocks separated by empty line. To avoid
	// unexpected sort, here remove all empty lines in import declaration.
	trimmed := trimImportEmptyLine(in)

	// This may potentially raise a false alerm. goimports actually adds
	// or removes some imports() entries, which depends on GOPATH.
	// However, this lint check is running outside of the chroot, unlike
	// actual build, so the GOPATH value and directory structure can be
	// different.
	out, err := runGoimports(trimmed)
	if err != nil {
		return []*Issue{&Issue{
			Pos: token.Position{Filename: path},
			Msg: err.Error(),
		}}
	}

	// Report the first different line if there is.
	inLines := bytes.Split(in, []byte("\n"))
	outLines := bytes.Split(out, []byte("\n"))
	for i, inLine := range inLines {
		outLine := outLines[i]
		if !bytes.Equal(inLine, outLine) {
			return []*Issue{&Issue{
				Pos: token.Position{Filename: path, Line: i + 1},
				Msg: "Import should be sorted in the following order: stdlib, third_party lib, chromiumos",
			}}
		}
	}

	// No issue is found.
	return nil
}

// trimImportEmptyLine removes empty lines in the import declaration.
func trimImportEmptyLine(in []byte) []byte {
	var lines [][]byte
	inImport := false
	for _, line := range bytes.Split(in, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)

		if inImport {
			if bytes.Equal(trimmed, []byte(")")) {
				inImport = false
			}
		} else {
			if bytes.Equal(trimmed, []byte("import (")) {
				inImport = true
			}
		}

		if inImport && len(trimmed) == 0 {
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
	cmd := exec.Command("goimports", "--local=chromiumos/")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	go func() {
		defer stdin.Close()
		stdin.Write(in)
	}()

	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}
