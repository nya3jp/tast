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
)

// ExternalJSON checks if url in .external file has a date suffix or not.
func ExternalJSON(path string, in []byte) []*Issue {
	// Ignore crostini.
	if strings.HasPrefix(filepath.Base(path), "crostini_trace_") {
		return nil
	}

	type DataFile struct {
		URL string `json:"url"`
	}
	var dataFile DataFile
	json.Unmarshal(in, &dataFile)
	url := dataFile.URL
	if url == "" {
		return nil
	}

	const date = `(?:.*(?:(?:20\d{2})(?:(?:(?:0[13578]|1[02])31)|(?:(?:0[1,3-9]|1[0-2])(?:29|30)))|(?:(?:20(?:0[48]|[2468][048]|[13579][26]))0229)|(?:20\d{2})(?:(?:0[1-9])|(?:1[0-2]))(?:[01][1-9]|10|2[0-8])).*$)`
	re := regexp.MustCompile(date)
	if !re.MatchString(url) {
		return []*Issue{{
			Pos:  token.Position{Filename: path},
			Msg:  fmt.Sprintf("include the date as a suffix in the filename like \"%s%s\"", strings.TrimRight(url, filepath.Ext(url))+"_YYYYMMDD", filepath.Ext(url)),
			Link: "https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#External-data-files",
		}}
	}
	return nil
}
