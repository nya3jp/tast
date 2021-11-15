// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package main implements tast-lint executable.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"go.chromium.org/tast/cmd/tast-lint/internal/check"
	"go.chromium.org/tast/cmd/tast-lint/internal/lint"
	"go.chromium.org/tast/shutil"
)

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

	issues, err := lint.Run(*commit, *debug, *fix, flag.Args())
	if err == lint.ErrNoTarget {
		flag.Usage()
		return
	}
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
