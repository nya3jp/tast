// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"chromiumos/cmd/tast-lint/check"
	"chromiumos/cmd/tast-lint/git"
)

// getTargetFiles returns the list of files to run lint according to flags.
func getTargetFiles(g *git.Git) ([]string, error) {
	if len(flag.Args()) == 0 && g.Commit != "" {
		return g.ModifiedFiles()
	}
	return flag.Args(), nil
}

// isTestFile checks if a file path is under Tast test directories.
func isTestFile(path string) bool {
	path, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	// Scripts and packages used for code generation are ignored since they're executed by "go run"
	// and can't easily include normal Tast packages.
	d := filepath.Dir(path)
	if strings.HasSuffix(d, "/gen") || strings.HasSuffix(d, "/genutil") {
		return false
	}

	return strings.Contains(path, "src/chromiumos/tast/local/") ||
		strings.Contains(path, "src/chromiumos/tast/remote/")
}

// isGoFile checks is a file is a Go code.
func isGoFile(path string) bool {
	return filepath.Ext(path) == ".go"
}

// hasFmtError runs gofmt to see if code has any formatting error.
func hasFmtError(code []byte, path string) bool {
	cmd := exec.Command("gofmt", "-l")
	cmd.Stdin = bytes.NewBuffer(code)
	out, err := cmd.Output()
	if err != nil {
		panic(fmt.Sprintf("Failed gofmt %s: %v", path, err))
	}
	return len(out) > 0
}

// checkAll runs all checks against paths.
func checkAll(g *git.Git, paths []string, debug bool) ([]*check.Issue, error) {
	cp := newCachedParser(g)
	fs := cp.fs

	var allIssues []*check.Issue
	for _, path := range paths {
		if !isGoFile(path) {
			continue
		}

		data, err := g.ReadFile(path)
		if err != nil {
			return nil, err
		}

		f, err := cp.parseFile(path)
		if err != nil {
			return nil, err
		}

		var issues []*check.Issue // issues in this file

		issues = append(issues, check.Golint(path, data, debug)...)
		if !hasFmtError(data, path) {
			// goimports applies gofmt, so skip it if the code has any formatting
			// error to avoid confusing reports. gofmt will be run by the repo
			// upload hook anyway.
			issues = append(issues, check.ImportOrder(path, data)...)
		}

		if isTestFile(path) {
			issues = append(issues, check.Declarations(fs, f)...)
			issues = append(issues, check.Exports(fs, f)...)
			issues = append(issues, check.ForbiddenCalls(fs, f)...)
			issues = append(issues, check.ForbiddenImports(fs, f)...)
			issues = append(issues, check.InterFileRefs(fs, f)...)
			issues = append(issues, check.Messages(fs, f)...)
		}

		// Only collect issues that weren't ignored by NOLINT comments.
		allIssues = append(allIssues, check.DropIgnoredIssues(issues, fs, f)...)
	}

	return allIssues, nil
}

// report prints issues to stdout.
func report(issues []*check.Issue) {
	check.SortIssues(issues)

	for _, i := range issues {
		fmt.Println(i)
	}

	linkSet := make(map[string]struct{})
	for _, i := range issues {
		if i.Link != "" {
			linkSet[i.Link] = struct{}{}
		}
	}
	if len(linkSet) > 0 {
		var links []string
		for link := range linkSet {
			links = append(links, link)
		}
		sort.Strings(links)

		fmt.Println()
		fmt.Println("Refer the following documents for details:")
		for _, link := range links {
			fmt.Println(" ", link)
		}
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

	g := git.New(".", *commit)

	files, err := getTargetFiles(g)
	if err != nil {
		panic(err)
	}
	if len(files) == 0 {
		flag.Usage()
		return
	}

	issues, err := checkAll(g, files, *debug)
	if err != nil {
		panic(err)
	}
	if len(issues) > 0 {
		report(issues)
		os.Exit(1)
	}
}
