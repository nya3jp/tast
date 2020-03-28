// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"chromiumos/tast/cmd/tast-lint/check"
	"chromiumos/tast/cmd/tast-lint/git"
	"chromiumos/tast/shutil"
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
		args = append(args, git.CommitFile{Status: status, Path: p})
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

// makeMapDirGoFile returns a map of directories and their committed Go files.
func makeMapDirGoFile(paths []git.CommitFile) map[string][]git.CommitFile {
	// Map from directory path to committed Go files in the dir.
	dfmap := make(map[string][]git.CommitFile)
	for _, path := range paths {
		if !strings.HasSuffix(path.Path, ".go") {
			continue
		}
		d := filepath.Dir(path.Path)
		dfmap[d] = append(dfmap[d], path)
	}
	return dfmap
}

// isNewlyAddedPackage returns true if given directory is a newly added package.
// The directory is a newly added package represents its all Go files were newly added (not modified or something else)
// at this commit.
// Thus, such directory should satisfy the following conditions:
//  - The number of ".go" files in the directory is equal to the number of ".go" files that were committed this time.
//  - Status of all committed ".go" files were "Added".
func isNewlyAddedPackage(g *git.Git, dir string, paths []git.CommitFile) (bool, error) {
	exfiles, err := g.ListDir(dir)
	if err != nil {
		return false, err
	}
	numExGofiles := 0
	for _, e := range exfiles {
		if !strings.HasSuffix(e, ".go") {
			continue
		}
		numExGofiles++
	}
	if numExGofiles != len(paths) {
		return false, nil
	}
	for _, path := range paths {
		if path.Status != git.Added {
			return false, nil
		}
	}
	return true, nil
}

// checkAll runs all checks against paths.
func checkAll(g *git.Git, paths []git.CommitFile, debug, fix bool) ([]*check.Issue, error) {
	cp := newCachedParser(g)
	fs := cp.fs

	var allIssues []*check.Issue

	var validPaths []git.CommitFile
	for _, path := range paths {
		if path.Status == git.Deleted || path.Status == git.TypeChanged ||
			path.Status == git.Unmerged || path.Status == git.Unknown {
			continue
		}
		validPaths = append(validPaths, path)
	}

	for _, path := range validPaths {
		if !strings.HasSuffix(path.Path, ".external") {
			continue
		}
		data, err := g.ReadFile(path.Path)
		if err != nil {
			continue
		}
		allIssues = append(allIssues, check.ExternalJSON(path.Path, data)...)
	}

	// Check secret variables naming convention
	for _, path := range validPaths {
		dir, file := filepath.Split(path.Path)
		if !strings.HasSuffix(dir, "vars/") || !strings.HasSuffix(file, ".yaml") {
			continue
		}
		data, err := g.ReadFile(path.Path)
		if err != nil {
			continue
		}
		allIssues = append(allIssues, check.SecretVarFile(path.Path, data)...)
	}

	dfmap := makeMapDirGoFile(validPaths)

	// Run individual file check in parallel.
	var mux sync.Mutex // guards fileIssues
	var fileIssues [][]*check.Issue
	eg, _ := errgroup.WithContext(context.Background())

	for dir, cfs := range dfmap {
		pkg, err := cp.parsePackage(dir)
		if err != nil {
			return nil, err
		}
		isNewlyAdded, err := isNewlyAddedPackage(g, dir, cfs)
		if err != nil {
			return nil, err
		}
		if isNewlyAdded {
			allIssues = append(allIssues, check.PackageComment(fs, pkg)...)
		}

		for _, path := range cfs {
			// Exempt protoc-generated Go files from lint checks.
			if strings.HasSuffix(path.Path, ".pb.go") {
				continue
			}
			f, ok := pkg.Files[path.Path] // take ast.File from parsed package
			if !ok {
				continue
			}

			mux.Lock()
			i := len(fileIssues)
			fileIssues = append(fileIssues, nil)
			mux.Unlock()
			path := path
			eg.Go(func() error {
				data, err := g.ReadFile(path.Path)
				if err != nil {
					return err
				}
				is, err := checkFile(path, data, debug, fs, f, fix)
				if err != nil {
					return err
				}
				mux.Lock()
				fileIssues[i] = is
				mux.Unlock()
				return nil
			})
		}
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	for _, issues := range fileIssues {
		allIssues = append(allIssues, issues...)
	}

	return allIssues, nil
}

// checkFile checks all the issues in the Go file in the given path. If fix is true, it automatically fixes f.
func checkFile(path git.CommitFile, data []byte, debug bool, fs *token.FileSet, f *ast.File, fix bool) ([]*check.Issue, error) {
	var issues []*check.Issue
	issues = append(issues, check.Golint(path.Path, data, debug)...)
	issues = append(issues, check.Comments(fs, f)...)
	issues = append(issues, check.EmptySlice(fs, f, fix)...)
	issues = append(issues, check.FuncParams(fs, f)...)

	if !hasFmtError(data, path.Path) {
		// goimports applies gofmt, so skip it if the code has any formatting
		// error to avoid confusing reports. gofmt will be run by the repo
		// upload hook anyway.
		if !fix {
			issues = append(issues, check.ImportOrder(path.Path, data)...)
		} else if newf, err := check.ImportOrderAutoFix(fs, f); err == nil {
			*f = *newf
		}
	}

	if isTestFile(path.Path) {
		issues = append(issues, check.Declarations(fs, f, fix)...)
		issues = append(issues, check.Exports(fs, f)...)
		issues = append(issues, check.ForbiddenBundleImports(fs, f)...)
		issues = append(issues, check.ForbiddenCalls(fs, f, fix)...)
		issues = append(issues, check.ForbiddenImports(fs, f)...)
		issues = append(issues, check.InterFileRefs(fs, f)...)
		issues = append(issues, check.Messages(fs, f, fix)...)
		issues = append(issues, check.VerifyTestingStateStruct(fs, f)...)
	}

	if isSupportPackageFile(path.Path) {
		issues = append(issues, check.VerifyTestingStateParam(fs, f)...)
	}

	if path.Status == git.Added {
		issues = append(issues, check.VerifyInformationalAttr(fs, f)...)
	}
	// Only collect issues that weren't ignored by NOLINT comments.
	issues = check.DropIgnoredIssues(issues, fs, f)

	// Format modified tree.
	if fix {
		if err := func() error {
			var buf bytes.Buffer
			if err := format.Node(&buf, fs, f); err != nil {
				return err
			}
			if hasFmtError(buf.Bytes(), "buffer") {
				return fmt.Errorf("failed gofmt")
			}
			tempfile, err := ioutil.TempFile(filepath.Dir(path.Path), "temp")
			if err != nil {
				return err
			}
			defer os.Remove(tempfile.Name())
			defer tempfile.Close()
			if _, err := buf.WriteTo(tempfile); err != nil {
				return err
			}
			if err := os.Rename(tempfile.Name(), path.Path); err != nil {
				return err
			}
			return nil
		}(); err != nil {
			return nil, err
		}
	}

	return issues, nil
}

// categorizeIssues categorize issues into auto-fixable and un-auto-fixable,
// then returns devided two slices.
func categorizeIssues(issues []*check.Issue) (fixable, unfixable []*check.Issue) {
	for _, i := range issues {
		if i.Fixable {
			fixable = append(fixable, i)
		} else {
			unfixable = append(unfixable, i)
		}
	}
	return
}

// report prints issues to stdout.
func report(issues []*check.Issue) {
	check.SortIssues(issues)

	for _, i := range issues {
		fmt.Println(" ", i)
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
		fmt.Println(" ", "Refer the following documents for details:")
		for _, link := range links {
			fmt.Println("  ", link)
		}
	}
}

func main() {
	commit := flag.String("commit", "", "if set, checks files in the specified Git commit")
	debug := flag.Bool("debug", false, "enables debug outputs")
	fix := flag.Bool("fix", false, "modifies auto-fixable errors automatically")
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

	issues, err := checkAll(g, files, *debug, *fix)
	if err != nil {
		panic(err)
	}
	if len(issues) > 0 && !*fix {
		// categorize issues
		fixable, unfixable := categorizeIssues(issues)
		if len(unfixable) > 0 {
			fmt.Println("Following errors should be modified by yourself:")
			report(unfixable)
			fmt.Println()
		}
		if len(fixable) > 0 {
			fmt.Println("Following errors can be automatically modified:")
			report(fixable)
			fmt.Println()
			cmd := append([]string{os.Args[0], "-fix"}, os.Args[1:]...)
			fmt.Printf("  You can run `%s` to fix this\n", shutil.EscapeSlice(cmd))
			fmt.Println()
		}
		os.Exit(1)
	}
}
