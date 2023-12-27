// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package check

import (
	"go/token"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/exp/slices"
)

func TestMainlineAttrsChecker(t *testing.T) {
	for name, item := range map[string]struct {
		attrs    []string
		wantMsgs []string
	}{
		"none": {},
		"only_mainline": {
			attrs: []string{"group:mainline"},
		},
		"informational_missing_mainline": {
			attrs:    []string{"informational"},
			wantMsgs: []string{informationalWithoutMainlineMsg},
		},
		"criticalstaging_missing_informational": {
			attrs:    []string{"group:mainline", "group:criticalstaging"},
			wantMsgs: []string{criticalstagingWithoutInformationalMsg},
		},
		"informational_criticalstaging_missing_mainline": {
			attrs: []string{"informational", "group:criticalstaging"},
			wantMsgs: []string{
				informationalWithoutMainlineMsg,
				criticalstagingWithoutInformationalMsg,
			},
		},
		"criticalstaging_missing_informational_and_mainline": {
			attrs:    []string{"group:criticalstaging"},
			wantMsgs: []string{criticalstagingWithoutInformationalMsg},
		},
		"informational_typo_with_group": {
			attrs:    []string{"group:mainline", "group:informational"},
			wantMsgs: []string{informationalWithGroupMsg},
		},
		"informational_typo_with_group_missing_mainline": {
			attrs: []string{"group:informational"},
			wantMsgs: []string{
				informationalWithGroupMsg,
				informationalWithoutMainlineMsg,
			},
		},
		"criticalstaging_typo_without_group": {
			attrs: []string{"group:mainline", "informational", "criticalstaging"},
			wantMsgs: []string{
				criticalStagingWithoutGroupMsg,
			},
		},
		"criticalstaging_typo_without_group_missing_mainline": {
			attrs: []string{"criticalstaging"},
			wantMsgs: []string{
				criticalStagingWithoutGroupMsg,
				criticalstagingWithoutInformationalMsg,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := mainlineAttrsChecker(item.attrs, token.Position{}, nil, token.Position{})
			var gotMsgs []string
			for _, issue := range got {
				gotMsgs = append(gotMsgs, issue.Msg)
			}
			slices.Sort(item.wantMsgs)
			slices.Sort(gotMsgs)
			if diff := cmp.Diff(gotMsgs, item.wantMsgs, cmpopts.EquateEmpty()); diff != "" {
				t.Error("Issues mismatch (-got +want): ", diff)
			}
		})
	}
}

func TestVerifyMainlineAttrs(t *testing.T) {
	const code = `package lacros
func init() {
	testing.AddTest(&testing.Test{
		Attr: []string{"group:mainline"},
		Params: []testing.Param{
			{
				Name: "p1",
			},
			{
				Name: "p2",
				ExtraAttr: []string{"informational"},
			},
			{
				Name: "p3",
				ExtraAttr: []string{"informational", "group:criticalstaging"},
			},
		},
	})
}
`
	const path = "/src/go.chromium.org/tast-tests/cros/local/parametrized.go"
	f, fs := parse(code, path)
	issues := VerifyMainlineAttrs(fs, f)
	verifyIssues(t, issues, nil)
}
