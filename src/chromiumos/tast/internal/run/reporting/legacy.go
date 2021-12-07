// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package reporting

import (
	"encoding/json"
	"os"

	"chromiumos/tast/internal/run/resultsjson"
)

// LegacyResultsFilename is a file name to be used with WriteLegacyResults.
const LegacyResultsFilename = "results.json"

// WriteLegacyResults writes results to path in the Tast's legacy results.json
// format.
func WriteLegacyResults(path string, results []*resultsjson.Result) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(results); err != nil {
		return err
	}
	return nil
}
