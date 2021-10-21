// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package lint implements the core part of tast-lint.
package lint

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"chromiumos/tast/errors"
	"go.chromium.org/tast/cmd/tast-lint/internal/check"
	"go.chromium.org/tast/cmd/tast-lint/internal/git"
)

// getTargetFiles returns the list of files to run lint according to flags.
func getTargetFiles(g *git.Git, deltaPath string, args []string) ([]git.CommitFile, error) {
	if len(args) == 0 {
		// If -commit is set, check changed files.
		if g.Commit != "" {
			return g.ChangedFiles()
		}

		// Otherwise, treat as if all files in the checkout were specified.
		args = nil // avoid clobbering args
		if err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeType != 0 {
				// Skip non-regular files.
				return nil
			}
			args = append(args, path)
			return nil
		}); err != nil {
			return nil, err
		}
	}

	currAbs, err := filepath.Abs(".")
	if err != nil {
		return nil, err
	}
	var files []git.CommitFile
	for _, p := range args {
		// If the CLI argument is an absolute path, creating filepath relative to git-root.
		if filepath.IsAbs(p) {
			p = filepath.Join(".", strings.TrimPrefix(p, currAbs))
		} else {
			p = filepath.Join(".", deltaPath, p)
		}
		files = append(files, git.CommitFile{Status: git.Modified, Path: p})
	}
	return files, nil
}

// isSupportPackageFile checks if a file path is of support packages.
func isSupportPackageFile(path string) bool {
	return isUserFile(path) &&
		!strings.Contains(path, "src/chromiumos/tast/local/bundles/") &&
		!strings.Contains(path, "src/chromiumos/tast/remote/bundles/")
}

// isUserFile checks if a file path is under the Tast user code directories.
func isUserFile(path string) bool {
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
		strings.Contains(path, "src/chromiumos/tast/remote/") ||
		strings.Contains(path, "src/chromiumos/tast/common/") ||
		strings.Contains(path, "src/chromiumos/tast/services/")
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
			if s, err := g.IsSymlink(path.Path); err != nil {
				return nil, err
			} else if s {
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
	issues = append(issues, check.FuncParams(fs, f, fix)...)
	issues = append(issues, check.DeprecatedAPIs(fs, f)...)
	issues = append(issues, check.FixtureDeclarations(fs, f, fix)...)

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

	if isUserFile(path.Path) {
		issues = append(issues, check.TestDeclarations(fs, f, path, fix)...)
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

// navigateGitRoot detects as well as change current directory to git root directory
// and returns the path difference between these two directories with error (if any).
func navigateGitRoot() (string, error) {
	// Relative path of the top-level directory relative to the current
	// directory (typically a sequence of "../", or an empty string)
	cmd := exec.Command("git", "rev-parse", "--show-cdup")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to locate git root: git rev-parse --show-cdup: %v", err)
	}

	gitRoot, err := filepath.Abs(strings.TrimRight(string(out), "\n"))
	if err != nil {
		return "", err
	}
	// Before changing directory, it's better to keep track of the current path for future use.
	currDir, err := filepath.Abs(".")
	if err != nil {
		return "", err
	}
	// The relative path difference between the git root and current path (considers symlinks too).
	deltaPath, err := filepath.Rel(gitRoot, currDir)
	if err != nil {
		return "", err
	}
	return deltaPath, os.Chdir(gitRoot)
}

// ErrNoTarget is returned by Run when there was no target to check.
var ErrNoTarget = errors.New("no target to check")

// Run runs lint checks and returns found issues without printing them to users.
func Run(commit string, debug, fix bool, args []string) ([]*check.Issue, error) {
	// Changing current directory to the Git root directory to aid the operations of git.go
	deltaPath, err := navigateGitRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to navigate to the git root directory")
	}

	g := git.New(".", commit)

	files, err := getTargetFiles(g, deltaPath, args)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, ErrNoTarget
	}

	return checkAll(g, files, debug, fix)
}
