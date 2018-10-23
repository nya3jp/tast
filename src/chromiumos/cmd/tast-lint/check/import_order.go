// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"bytes"
	"errors"
	"fmt"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"
)

// ImportOrder checks if the order of import entries are sorted in the
// following order.
//   - Import entries should be split into three groups; stdlib, third-party
//     packages, and chromiumos packages.
//   - In each group, the entries should be sorted in the lexicographical
//     order.
//   - The groups should be separated by an empty line.
// This order should be same as what "goimports --local chromiumos/" does.
func ImportOrder(path string, in []byte) []*Issue {
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

	if !bytes.Equal(in, out) {
		diff, err := runDiff(in, out)
		if err != nil {
			panic(err.Error())
		}

		return []*Issue{{
			Pos:  token.Position{Filename: path},
			Msg:  fmt.Sprintf("Import should be grouped into standard packages, third-party packages and chromiumos packages in this order separated by empty lines.\nApply the following patch to fix:\n%s", diff),
			Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#import",
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

// writeTempFile creates a temp file containing the given data by using
// the given name. Returns the name of the created temp file.
func writeTempFile(data []byte) (string, error) {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// runDiff takes the diff between orig and expect, and returns the diff in
// unified diff format.
func runDiff(orig []byte, expect []byte) (string, error) {
	f1, err := writeTempFile(orig)
	if err != nil {
		return "", err
	}
	defer os.Remove(f1)

	f2, err := writeTempFile(expect)
	if err != nil {
		return "", err
	}
	defer os.Remove(f2)

	// Ignore error. diff command returns error if difference is found.
	out, _ := exec.Command("diff", "-u", f1, f2).CombinedOutput()

	// Strip leading two lines, which are temp file name.
	parts := bytes.SplitN(out, []byte("\n"), 3)
	if len(parts) < 3 {
		return "", errors.New("Unexpected diff output")
	}
	return string(parts[2]), nil
}
