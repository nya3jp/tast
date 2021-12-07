// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package reporting

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"chromiumos/tast/internal/logging"
	"chromiumos/tast/internal/run/resultsjson"
	"chromiumos/tast/internal/testing"
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

	// Setup results variables with ansi color.
	passStr := " [ PASS ]"
	skipStr := " [ SKIP ]"
	failStr := " [ FAIL ] "
	notRunStr := " [NOTRUN] "
	const RED = "\033[1;31m"
	const GREEN = "\033[1;32m"
	const YELLOW = "\033[1;33m"
	const MAGENTA = "\033[1;35m"
	const RESET = "\033[0m"
	passStrClr := fmt.Sprintf("%v [ PASS ] %v", GREEN, RESET)
	skipStrClr := fmt.Sprintf("%v [ SKIP ] %v", YELLOW, RESET)
	failStrClr := fmt.Sprintf("%v [ FAIL ] %v", RED, RESET)
	notRunStrClr := fmt.Sprintf("%v [NOTRUN] %v", MAGENTA, RESET)
	t := time.Now()
	timeStr := t.UTC().Format("2006-01-02T15:04:05.000000Z")

	for _, res := range results {
		pn := fmt.Sprintf("%-"+strconv.Itoa(ml)+"s", res.Name)
		if len(res.Errors) == 0 {
			if res.SkipReason == "" {
				logging.Debug(ctx, pn+passStr)
				fmt.Printf("%v %v\n", timeStr, pn+passStrClr)
			} else {
				logging.Debug(ctx, pn+skipStr)
				fmt.Printf("%v %v\n", timeStr, pn+skipStrClr+res.SkipReason)
			}
		} else {
			for i, te := range res.Errors {
				if i == 0 {
					if te.Reason == testing.TestDidNotRunMsg {
						logging.Debug(ctx, pn+notRunStr+te.Reason)
						fmt.Printf("%v %v\n", timeStr, pn+notRunStrClr+te.Reason)
					} else {
						logging.Debug(ctx, pn+failStr+te.Reason)
						fmt.Printf("%v %v\n", timeStr, pn+failStrClr+te.Reason)
					}
				} else {
					logging.Debug(ctx, strings.Repeat(" ", ml+len(failStr))+te.Reason)
					fmt.Printf("%v %v\n", timeStr, strings.Repeat(" ", ml+len(failStr))+te.Reason)
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
