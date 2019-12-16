// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"encoding/json"
	"fmt"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// dateSuffixRe is a compiled regular expression for date suffix check.
// The filename stem should end with YYYYMMDD date pattern, or in addition
// _RC\d+ pattern version suffix.
var dateSuffixRe = regexp.MustCompile(`(?:[-_])(\d{8})(?:_RC\d+)?(?:[-.]|$)`)

// ExternalJSON checks if url in .external file has a date suffix or not.
func ExternalJSON(path string, in []byte) []*Issue {
	type DataFile struct {
		URL string `json:"url"`
	}
	var dataFile DataFile
	if err := json.Unmarshal(in, &dataFile); err != nil {
		return nil
	}
	url := dataFile.URL
	if !strings.HasPrefix(url, "gs://chromiumos-test-assets-public/tast/") && !strings.HasPrefix(url, "gs://chromeos-test-assets-private/tast/") {
		return nil
	}

	// Ignore crostini because it manages version with some numbers, not the date suffix.
	crostiniPrefixList := []string{
		"gs://chromiumos-test-assets-public/tast/cros/vm/termina_kernel_aarch64_",
		"gs://chromiumos-test-assets-public/tast/cros/vm/termina_kernel_x86_64_",
		"gs://chromiumos-test-assets-public/tast/cros/vm/termina_rootfs_aarch64_",
		"gs://chromiumos-test-assets-public/tast/cros/vm/termina_rootfs_x86_64_",
		"gs://chromeos-test-assets-private/tast/crosint/graphics/traces/",
	}
	for _, prefix := range crostiniPrefixList {
		if strings.HasPrefix(url, prefix) {
			return nil
		}
	}

	base := filepath.Base(url)
	match := dateSuffixRe.FindStringSubmatch(base)
	if match != nil {
		if _, err := time.Parse("20060102", match[1]); err == nil {
			return nil
		}
	}

	ext := filepath.Ext(base)
	return []*Issue{{
		Pos:  token.Position{Filename: path},
		Msg:  fmt.Sprintf("include the date as a suffix in the filename like \".../%s_YYYYMMDD%s\"", strings.TrimRight(base, ext), ext),
		Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#External-data-files",
	}}
}
