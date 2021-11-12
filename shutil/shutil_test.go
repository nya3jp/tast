// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package shutil_test

import (
	"testing"

	"go.chromium.org/tast/shutil"
)

func TestEscape(t *testing.T) {
	for _, c := range []struct {
		in, exp string
	}{
		{``, `''`},
		{` `, `' '`},
		{`\t`, `'\t'`},
		{`\n`, `'\n'`},
		{`ab`, `ab`},
		{`a b`, `'a b'`},
		{`ab `, `'ab '`},
		{` ab`, `' ab'`},
		{`AZaz09@%_+=:,./-`, `AZaz09@%_+=:,./-`},
		{`a!b`, `'a!b'`},
		{`'`, `''"'"''`},
		{`"`, `'"'`},
		{`=foo`, `'=foo'`},
		{`Tast's`, `'Tast'"'"'s'`},
	} {
		if s := shutil.Escape(c.in); s != c.exp {
			t.Errorf("Escape(%q) = %q; want %q", c.in, s, c.exp)
		}
	}
}
