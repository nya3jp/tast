// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"encoding/json"
	"io"
	"path/filepath"

	"chromiumos/tast/common/testing"
)

// listDataFiles writes paths for data files required by testing.Test objects in ts
// to w as a JSON array of strings. Paths are relative to the top-level data directory.
func listDataFiles(w io.Writer, ts []*testing.Test, arch string) error {
	paths := make([]string, 0)
	seen := make(map[string]struct{})
	for _, t := range ts {
		if t.Data == nil {
			continue
		}
		for _, p := range t.Data {
			p = filepath.Join(t.DataDir(), testing.TestDataPathForArch(p, arch))
			if _, ok := seen[p]; !ok {
				paths = append(paths, p)
				seen[p] = struct{}{}
			}
		}
	}
	b, err := json.Marshal(paths)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}
