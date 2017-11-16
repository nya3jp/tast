// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package run

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"chromiumos/tast/common/testing"
)

// printTests writes the names of tests in b, a JSON-marshaled array of
// testing.Test structs, to w.
func printTests(w io.Writer, b []byte, mode PrintMode) error {
	ts := make([]testing.Test, 0)
	if err := json.Unmarshal(b, &ts); err != nil {
		return err
	}

	switch mode {
	case PrintNames:
		for _, t := range ts {
			if _, err := fmt.Fprintf(w, "%s\n", t.Name); err != nil {
				return err
			}
		}
		return nil
	case PrintJSON:
		// Pretty-print the tests back to JSON.
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(ts)
	default:
		return errors.New("invalid print mode")
	}
}
