// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package command

// CopyFieldIfNonZero copies *src to *dst if *src does not contain the zero value.
// This is intended to be used when deprecating fields in structs used for IPC.
// src and dst must be of the same type; *bool, *string, and *[]string are supported.
// This function panics if passed any other types.
func CopyFieldIfNonZero(src, dst interface{}) {
	switch s := src.(type) {
	case *bool:
		if *s {
			*dst.(*bool) = *s
		}
	case *string:
		if *s != "" {
			*dst.(*string) = *s
		}
	case *[]string:
		if *s != nil {
			*dst.(*[]string) = *s
		}
	default:
		panic("Unsupported deprecated field type")
	}
}
