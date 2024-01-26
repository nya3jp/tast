// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"errors"
	"strings"
	"testing"
)

func TestNewErrorInvalidUTF8(t *testing.T) {
	const invalidUTF8 = "\xed"
	for _, tc := range []struct {
		msg    string
		expect string
	}{
		{
			msg:    "normal error",
			expect: "normal error",
		},
		{
			msg:    "error with invalid UTF-8 character " + invalidUTF8,
			expect: "error with invalid UTF-8 character ",
		},
	} {
		errProto := NewError(errors.New(tc.msg), tc.msg, tc.msg, 0)
		if errProto.GetReason() != tc.expect {
			t.Errorf("NewError returned reason %q; want %q", errProto.GetReason(), tc.expect)
		}
		if strings.Contains(errProto.GetLocation().GetStack(), invalidUTF8) {
			t.Errorf("NewError returned stack %q that include invalid string", errProto.GetLocation().GetStack())

		}
	}
}
