// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/ast"
	"go/token"
	"sort"
	"strings"
)

// Issue holds an issue reported by the linter.
type Issue struct {
	Pos     token.Position
	Msg     string
	Link    string
	Fixable bool
}

func (i *Issue) String() string {
	return fmt.Sprintf("%s: %s", i.Pos, i.Msg)
}

// SortIssues sorts issues by file path and position.
func SortIssues(issues []*Issue) {
	sort.Slice(issues, func(i, j int) bool {
		pi := issues[i].Pos
		pj := issues[j].Pos
		if pi.Filename != pj.Filename {
			return pi.Filename < pj.Filename
		}
		return pi.Offset < pj.Offset
	})
}

// DropIgnoredIssues drops all issues that are on the same lines as NOLINT comments.
//
// Specifically, an issue is dropped if its line number matches the starting line
// number of a comment group (see ast.CommentGroup) in f that contains "NOLINT".
//
// Filtering is performed at this level rather than while walking the AST for several reasons:
//  - We want to avoid making each check look for NOLINT itself.
//  - We can't just skip nodes that are on the same lines as NOLINT comments, since some issues are
//    reported with different line numbers than the position of the node from which they were reported.
//    For example, the Declarations function inspects testing.Test composite literal nodes,
//    but the line numbers used in its issues correspond to Desc fields that contain errors.
//    We expect test authors to place a NOLINT comment at the end of the line containing the Desc field,
//    not on the line containing the beginning of the testing.Test literal.
func DropIgnoredIssues(issues []*Issue, fs *token.FileSet, f *ast.File) []*Issue {
	nolintLineNums := make(map[int]struct{})
	for _, cg := range f.Comments {
		if strings.Contains(cg.Text(), "NOLINT") {
			lineNum := fs.File(cg.Pos()).Line(cg.Pos())
			nolintLineNums[lineNum] = struct{}{}
		}
	}

	var kept []*Issue
	for _, issue := range issues {
		if _, ok := nolintLineNums[issue.Pos.Line]; !ok {
			kept = append(kept, issue)
		}
	}
	return kept
}
