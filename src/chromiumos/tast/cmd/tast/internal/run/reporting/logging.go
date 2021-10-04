// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package reporting

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"chromiumos/tast/cmd/tast/internal/run/resultsjson"
	"chromiumos/tast/internal/logging"
)

// WriteResultsToLogs writes test results to the console via ctx.
// resDir is the directory where test result files have been saved. complete
// indicates whether we could run all tests.
func WriteResultsToLogs(ctx context.Context, results []*resultsjson.Result, resDir string, complete bool) {
	ml := 0
	for _, res := range results {
		if len(res.Name) > ml {
			ml = len(res.Name)
		}
	}

	sep := strings.Repeat("-", 80)
	logging.Info(ctx, sep)

	for _, res := range results {
		pn := fmt.Sprintf("%-"+strconv.Itoa(ml)+"s", res.Name)
		if len(res.Errors) == 0 {
			if res.SkipReason == "" {
				logging.Info(ctx, pn+"  [ PASS ]")
			} else {
				logging.Info(ctx, pn+"  [ SKIP ] "+res.SkipReason)
			}
		} else {
			const failStr = "  [ FAIL ] "
			for i, te := range res.Errors {
				if i == 0 {
					logging.Info(ctx, pn+failStr+te.Reason)
				} else {
					logging.Info(ctx, strings.Repeat(" ", ml+len(failStr))+te.Reason)
				}
			}
		}
	}

	if !complete {
		// If the run didn't finish, log an additional message after the individual results
		// to make it clearer that all is not well.
		logging.Info(ctx, "")
		logging.Info(ctx, "Run did not finish successfully; results are incomplete")
	}

	logging.Info(ctx, sep)
	logging.Info(ctx, "Results saved to ", resDir)
}
