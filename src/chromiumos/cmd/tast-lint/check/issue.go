// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"fmt"
	"go/token"
)

// Issue holds an issue reported by the linter.
type Issue struct {
	Pos token.Pos
	Msg string
}

func (i *Issue) String(fs *token.FileSet) string {
	return fmt.Sprintf("%s: %s", fs.Position(i.Pos), i.Msg)
}
