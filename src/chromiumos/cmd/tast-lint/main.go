// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"chromiumos/cmd/tast-lint/check"
)

// getTargetFiles returns the list of files to run lint according to flags.
func getTargetFiles(git *git) ([]string, error) {
	if len(flag.Args()) == 0 && git.commit != "" {
		return git.modifiedFiles()
	}
	return flag.Args(), nil
}

// isTestFile checks if a file path is under Tast test directories.
func isTestFile(path string) bool {
	path, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return strings.Contains(path, "src/chromiumos/tast/local/") ||
		strings.Contains(path, "src/chromiumos/tast/remote/")
}

// isGoFile checks is a file is a Go code.
func isGoFile(path string) bool {
	return filepath.Ext(path) == ".go"
}

// checkAll runs all checks against paths.
func checkAll(git *git, paths []string, debug bool) ([]*check.Issue, error) {
	fs := token.NewFileSet()

	var issues []*check.Issue
	for _, path := range paths {
		if isGoFile(path) && isTestFile(path) {
			data, err := git.readFile(path)
			if err != nil {
				return nil, err
			}

			f, err := parser.ParseFile(fs, path, data, 0)
			if err != nil {
				return nil, err
			}

			issues = append(issues, check.ErrorsImports(fs, f)...)
			issues = append(issues, check.FmtErrorf(fs, f)...)
			issues = append(issues, check.Golint(path, data, debug)...)
			issues = append(issues, check.ImportOrder(path, data)...)
		}
	}

	return issues, nil
}

// report prints issues to stdout.
func report(issues []*check.Issue) {
	sort.Slice(issues, func(i, j int) bool {
		pi := issues[i].Pos
		pj := issues[j].Pos
		if pi.Filename != pj.Filename {
			return pi.Filename < pj.Filename
		}
		return pi.Offset < pj.Offset
	})

	for _, i := range issues {
		fmt.Println(i)
	}
}

func main() {
	commit := flag.String("commit", "", "if set, checks files in the specified Git commit")
	debug := flag.Bool("debug", false, "enables debug outputs")
	flag.Parse()

	// TODO(nya): Allow running lint from arbitrary directories.
	// Currently git.go assumes the current directory is a Git root directory.
	if _, err := os.Stat(".git"); err != nil {
		panic("This tool can be run at a Git root directory only")
	}

	git := newGit(*commit)

	files, err := getTargetFiles(git)
	if err != nil {
		panic(err)
	}
	if len(files) == 0 {
		flag.Usage()
		return
	}

	issues, err := checkAll(git, files, *debug)
	if err != nil {
		panic(err)
	}
	if len(issues) > 0 {
		report(issues)
		os.Exit(1)
	}
}
