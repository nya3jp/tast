// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devserver_test

import (
	"testing"

	"chromiumos/tast/internal/devserver"
)

func TestParseGSURL(t *testing.T) {
	for _, c := range []struct {
		url          string
		ok           bool
		bucket, path string
	}{
		{"", false, "", ""},
		{"http://example.com/", false, "", ""},
		{"gs://bucket/path/to/file", true, "bucket", "path/to/file"},
		{"gs://bucket/path/to/dir/", true, "bucket", "path/to/dir/"},
		{"gs://bucket/%2521", true, "bucket", "%21"},
		{"gs://bucket", true, "bucket", ""},
		{"gs://bucket/", true, "bucket", ""},
	} {
		bucket, path, err := devserver.ParseGSURL(c.url)
		if ok := err == nil; ok != c.ok {
			if c.ok {
				t.Errorf("ParseGSURL(%q) unexpectedly failed: %v", c.url, err)
			} else {
				t.Errorf("ParseGSURL(%q) unexpectedly succeeded: (%q, %q)", c.url, bucket, path)
			}
		} else if ok && (bucket != c.bucket || path != c.path) {
			t.Errorf("ParseGSURL(%q) = (%q, %q); want (%q, %q)", c.url, bucket, path, c.bucket, c.path)
		}
	}
}
