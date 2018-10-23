// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/token"
	"sort"
)

// Issue holds an issue reported by the linter.
type Issue struct {
	Pos token.Position
	Msg string
}

func (i *Issue) String() string {
	return fmt.Sprintf("%s: %s", i.Pos, i.Msg)
}

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
