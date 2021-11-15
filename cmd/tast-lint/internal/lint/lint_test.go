// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lint_test

import (
	"os"
	"os/exec"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"go.chromium.org/tast/cmd/tast-lint/internal/lint"
	"go.chromium.org/tast/testutil"
)

// setUpGitRepo creates a new Git repository in a temporary directory and sets
// the current directory there. It calls testing.T.Cleanup to restore the
// current directory on finishing the current test.
func setUpGitRepo(t *testing.T) {
	repoDir := testutil.TempDir(t)
	t.Cleanup(func() { os.RemoveAll(repoDir) })

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	os.Chdir(repoDir)
	t.Cleanup(func() { os.Chdir(origDir) })

	// Set up a new Git repo.
	if err := exec.Command("git", "init").Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	for _, kv := range []struct {
		key, value string
	}{
		{"user.name", "me"},
		{"user.email", "me@example.com"},
	} {
		if err := exec.Command("git", "config", "--local", kv.key, kv.value).Run(); err != nil {
			t.Fatalf("git config failed: %v", err)
		}
	}

	// Create a first empty commit. This is required because ChangedFiles
	// does not work with a parent-less commit.
	if err := exec.Command("git", "commit", "-m", "init", "--allow-empty").Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}
}

// TestRun_TargetSelection checks which files are selected by different specs.
// See b/197290276.
func TestRun_TargetSelection(t *testing.T) {
	setUpGitRepo(t)

	// Create a commit containing two bad files.
	const badCode = "package pkg\n// This is bad comment\nfunc init() {}\n"
	if err := testutil.WriteFiles(".", map[string]string{
		"src/chromiumos/tast/testing/aaa.go": badCode,
		"src/chromiumos/tast/testing/bbb.go": badCode,
	}); err != nil {
		t.Fatalf("Failed to write files: %v", err)
	}
	if err := exec.Command("git", "add",
		"src/chromiumos/tast/testing/aaa.go",
		"src/chromiumos/tast/testing/bbb.go").Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if err := exec.Command("git", "commit", "-m", "commit").Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	// Write some files without committing.
	const goodCode = "package pkg\n"
	if err := testutil.WriteFiles(".", map[string]string{
		// Overwrite aaa.go with good contents.
		"src/chromiumos/tast/testing/aaa.go": goodCode,
		// Create ccc.go with bad contents.
		"src/chromiumos/tast/testing/ccc.go": badCode,
	}); err != nil {
		t.Fatalf("Failed to write files: %v", err)
	}

	for _, tc := range []struct {
		commit string
		args   []string
		want   map[string]struct{}
	}{
		{
			// No commit and no args: check all files in the checkout.
			commit: "",
			args:   nil,
			want: map[string]struct{}{
				"src/chromiumos/tast/testing/bbb.go": {},
				"src/chromiumos/tast/testing/ccc.go": {},
			},
		},
		{
			// Commit specified: check all files modified in the commit.
			commit: "HEAD",
			args:   nil,
			want: map[string]struct{}{
				"src/chromiumos/tast/testing/aaa.go": {},
				"src/chromiumos/tast/testing/bbb.go": {},
			},
		},
		{
			// Args specified: check specified files.
			commit: "",
			args: []string{
				"src/chromiumos/tast/testing/aaa.go",
				"src/chromiumos/tast/testing/ccc.go",
			},
			want: map[string]struct{}{
				"src/chromiumos/tast/testing/ccc.go": {},
			},
		},
		{
			// Both commit and args specified: check specified files as of the commit.
			commit: "HEAD",
			args: []string{
				"src/chromiumos/tast/testing/aaa.go",
				"src/chromiumos/tast/testing/bbb.go",
			},
			want: map[string]struct{}{
				"src/chromiumos/tast/testing/aaa.go": {},
				"src/chromiumos/tast/testing/bbb.go": {},
			},
		},
	} {
		issues, err := lint.Run(tc.commit, false, false, tc.args)
		if err == lint.ErrNoTarget {
			issues = nil
		} else if err != nil {
			t.Errorf("Run(commit=%q, args=%q) failed: %v", tc.commit, tc.args, err)
			continue
		}
		got := make(map[string]struct{})
		for _, issue := range issues {
			got[issue.Pos.Filename] = struct{}{}
		}
		if diff := cmp.Diff(got, tc.want, cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("Run(commit=%q, args=%q) mismatch (-got +want):\n%s", tc.commit, tc.args, diff)
		}
	}
}

// TestRun_FileCategories ensures files are categorized expectedly.
// See b/197290278.
func TestRun_FileCategories(t *testing.T) {
	setUpGitRepo(t)

	for _, tc := range []struct {
		check   string
		content string
		want    map[string]struct{}
	}{
		{
			// testing.State check should be applied to support library files only.
			check: "testing.State check",
			content: `package pkg
import "chromiumos/tast/testing"
func f(s *testing.State) {}
`,
			want: map[string]struct{}{
				"src/chromiumos/tast/local/chrome/chrome.go":      {},
				"src/chromiumos/tast/remote/chrome/chrome.go":     {},
				"src/chromiumos/tast/common/chrome/chrome.go":     {},
				"src/chromiumos/tast/services/cros/chrome/gen.go": {},
			},
		},
		{
			// errors import check should be applied to user code only.
			check: "errors import check",
			content: `package pkg
import "errors"
func f() error { return errors.New("hello") }
`,
			want: map[string]struct{}{
				"src/chromiumos/tast/local/bundles/cros/ui/cuj/cuj.go":  {},
				"src/chromiumos/tast/remote/bundles/cros/ui/cuj/cuj.go": {},
				"src/chromiumos/tast/local/chrome/chrome.go":            {},
				"src/chromiumos/tast/remote/chrome/chrome.go":           {},
				"src/chromiumos/tast/common/chrome/chrome.go":           {},
				"src/chromiumos/tast/services/cros/chrome/gen.go":       {},
			},
		},
	} {
		// Write the same content to many files.
		if err := testutil.WriteFiles(".", map[string]string{
			// Test bundle files.
			"src/chromiumos/tast/local/bundles/cros/ui/cuj/cuj.go":  tc.content,
			"src/chromiumos/tast/remote/bundles/cros/ui/cuj/cuj.go": tc.content,
			// Support library files.
			"src/chromiumos/tast/local/chrome/chrome.go":                    tc.content,
			"src/chromiumos/tast/remote/chrome/chrome.go":                   tc.content,
			"src/chromiumos/tast/common/chrome/chrome.go":                   tc.content,
			"src/chromiumos/tast/services/cros/chrome/gen.go":               tc.content,
			"src/chromiumos/tast/services/cros/chrome/chrome_service.pb.go": tc.content,
			// Framework files.
			"src/chromiumos/tast/errors/errors.go":   tc.content,
			"src/chromiumos/tast/testing/testing.go": tc.content,
			// Unrelated files.
			"src/chromiumos/infra/infra.go": tc.content,
			"tools/main.go":                 tc.content,
		}); err != nil {
			t.Fatalf("Failed to write files: %v", err)
		}

		issues, err := lint.Run("", false, false, nil)
		if err != nil {
			t.Errorf("Run failed for %s: %v", tc.check, err)
			continue
		}

		got := make(map[string]struct{})
		for _, issue := range issues {
			got[issue.Pos.Filename] = struct{}{}
		}
		if diff := cmp.Diff(got, tc.want, cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("Run mismatch for %s (-got +want):\n%s", tc.check, diff)
		}
	}
}
