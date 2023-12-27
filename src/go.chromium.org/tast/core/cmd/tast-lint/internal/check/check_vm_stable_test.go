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

func TestVMStableAttrsChecker(t *testing.T) {
	for name, item := range map[string]struct {
		attrs    []string
		wantMsgs []string
	}{
		"none": {},
		"expected": {
			attrs: []string{"group:hw_agnostic", "group:mainline", "informational", "hw_agnostic_vm_stable"},
		},
		"vm_stable_missing_hw_agnostic": {
			attrs:    []string{"hw_agnostic_vm_stable", "group:mainline", "informational"},
			wantMsgs: []string{vmStableWithoutHWAgnosticMsg},
		},
		"vm_stable_missing_mainline": {
			attrs:    []string{"hw_agnostic_vm_stable", "group:hw_agnostic", "informational"},
			wantMsgs: []string{vmStableWithoutMainlineMsg},
		},
		"vm_stable_missing_informational": {
			attrs:    []string{"hw_agnostic_vm_stable", "group:hw_agnostic", "group:mainline"},
			wantMsgs: []string{vmStableWithoutInformationalMsg},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := vmStableAttrChecker(item.attrs, token.Position{}, nil, token.Position{})
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
