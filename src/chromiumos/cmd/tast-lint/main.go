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
func getTargetFiles(g *git.Git) ([]git.CommitFile, error) {
	if len(flag.Args()) == 0 && g.Commit != "" {
		return g.ChangedFiles()
	}

	var statusStr string
	flag.StringVar(&statusStr, "status", "M", "File Status")
	var status git.CommitStatus
	switch statusStr {
	case "A":
		status = git.Added
	case "C":
		status = git.Copied
	case "D":
		status = git.Deleted
	case "M":
		status = git.Modified
	case "R":
		status = git.Renamed
	case "T":
		status = git.TypeChanged
	case "U":
		status = git.Unmerged
	case "X":
		status = git.Unknown
	default:
		return nil, fmt.Errorf("please input valid status")
	}
	var args []git.CommitFile
	for _, p := range flag.Args() {
		args = append(args, git.CommitFile{status, p})
	}
	return args, nil
}

// isSupportPackageFile checks if a file path is of support packages.
func isSupportPackageFile(path string) bool {
	return isTestFile(path) &&
		!strings.Contains(path, "src/chromiumos/tast/local/bundles/") &&
		!strings.Contains(path, "src/chromiumos/tast/remote/bundles/")
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
func checkAll(g *git.Git, paths []git.CommitFile, debug bool) ([]*check.Issue, error) {
	cp := newCachedParser(g)
	fs := cp.fs

	var allIssues []*check.Issue
	for _, path := range paths {
		if path.Status == git.Deleted || path.Status == git.TypeChanged ||
			path.Status == git.Unmerged || path.Status == git.Unknown {
			continue
		}

		if !strings.HasSuffix(path.Path, ".go") {
			continue
		}
		// Exempt protoc-generated Go files from lint checks.
		if strings.HasSuffix(path.Path, ".pb.go") {
			continue
		}

		data, err := g.ReadFile(path.Path)
		if err != nil {
			return nil, err
		}

		f, err := cp.parseFile(path.Path)
		if err != nil {
			return nil, err
		}

		var issues []*check.Issue // issues in this file

		issues = append(issues, check.Golint(path.Path, data, debug)...)
		issues = append(issues, check.Comments(fs, f)...)
		issues = append(issues, check.EmptySlice(fs, f)...)

		if !hasFmtError(data, path.Path) {
			// goimports applies gofmt, so skip it if the code has any formatting
			// error to avoid confusing reports. gofmt will be run by the repo
			// upload hook anyway.
			issues = append(issues, check.ImportOrder(path.Path, data)...)
		}

		if isTestFile(path.Path) {
			issues = append(issues, check.Declarations(fs, f)...)
			issues = append(issues, check.Exports(fs, f)...)
			issues = append(issues, check.ForbiddenBundleImports(fs, f)...)
			issues = append(issues, check.ForbiddenCalls(fs, f)...)
			issues = append(issues, check.ForbiddenImports(fs, f)...)
			issues = append(issues, check.InterFileRefs(fs, f)...)
			issues = append(issues, check.Messages(fs, f)...)
			issues = append(issues, check.VerifyTestingStateStruct(fs, f)...)
		}

		if isSupportPackageFile(path.Path) {
			issues = append(issues, check.VerifyTestingStateParam(fs, f)...)
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
