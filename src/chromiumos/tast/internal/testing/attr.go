// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
	"strings"
)

// group defines a group of tests having the same purpose. For example, the mainline
// group contains functional tests to be used for build verification.
//
// A group definition should be agnostic to how it is scheduled in infrastructure.
// If needed, extra attributes can be defined to give hints for scheduling.
type group struct {
	// Name is the name of the group. A test can declare to belong to a group by
	// adding an attribute "group:<name>".
	Name string

	// Contacts is a list of email addresses of persons and groups responsible for
	// maintaining the test group.
	Contacts []string

	// Desc is a description of the group.
	Desc string

	// Subattrs defines extra attributes that can be used to annotate the tests
	// in the group.
	Subattrs []*attr
}

// attr defines an extra attribute to annotate tests.
//
// Attributes can give hint for scheduling.
type attr struct {
	// Name is the name of the attribute.
	Name string

	// Desc is a description of the attribute.
	Desc string
}

// validGroups is the list of all valid groups.
var validGroups = []*group{
	{
		Name:     "mainline",
		Contacts: []string{"tast-owners@google.com"},
		Desc: `The default group of functional tests.

Mainline tests are run for build verification. Among others, pre-submit and
post-submit testing in Chrome OS CI and Chromium CI are important places where
mainlines tests are run.
`,
		Subattrs: []*attr{
			{
				Name: "informational",
				Desc: `Indicates that failures can be ignored.

Mainline tests lacking this attribute are called critical tests. Failures in
critical tests justify rejecting or reverting the responsible change, while
failures in informational tests do not.

All mainline tests should be initially marked informational, and should be
promoted to critical tests after stabilization.
`,
			},
		},
	},
	{
		Name:     "crosbolt",
		Contacts: []string{"crosbolt-eng@google.com"},
		Desc: `The group of performance tests to be run regularly by the crosbolt team.

Tests in this group are not used for build verification.
`,
		Subattrs: []*attr{
			{
				Name: "crosbolt_perbuild",
				Desc: `Indicates that this test should run for every Chrome OS build.`,
			},
			{
				Name: "crosbolt_nightly",
				Desc: `Indicates that this test should run nightly.`,
			},
			{
				Name: "crosbolt_weekly",
				Desc: `Indicates that this test should run weekly.`,
			},
			{
				Name: "crosbolt_memory_nightly",
				Desc: `Indicates that this test is a memory test and should run nightly.`,
			},
		},
	},
	{
		Name:     "graphics",
		Contacts: []string{"chromeos-gfx@google.com", "chromeos-gfx-video@google.com"},
		Desc: `The group of graphics tests to be run regularly by the graphics team.

Tests in this group are not used for build verification.
`,
		Subattrs: []*attr{
			{
				Name: "graphics_trace",
				Desc: `Indicate this test is replaying a trace to reproduce graphics command.`,
			},
			{
				Name: "graphics_video",
				Desc: `Indicate this test is focus on video encode/decode.`,
			},
			{
				Name: "graphics_perbuild",
				Desc: `Indicates that this test should run for every Chrome OS build.`,
			},
			{
				Name: "graphics_nightly",
				Desc: `Indicates that this test should run nightly.`,
			},
			{
				Name: "graphics_weekly",
				Desc: `Indicates that this test should run weekly.`,
			},
			{
				Name: "graphics_av_analysis",
				Desc: `Indicates that this test should run on audio/video analysis pool.`,
			},
		},
	},
	{
		Name:     "stress",
		Contacts: []string{"chromeos-engprod@google.com"},
		Desc:     `A group of stress tests.`,
	},
	{
		Name:     "arc-data-collector",
		Contacts: []string{"chromeos-engprod@google.com"},
		Desc:     `A group of ARC tests to be run in Android PFQ and collect data for specific Android build.`,
	},
	{
		Name:     "appcompat",
		Contacts: []string{"chromeos-engprod@google.com"},
		Desc:     `A group of ARC app compatibility tests.`,
	},
	{
		Name:     "enrollment",
		Contacts: []string{"vsavu@google.com", "enterprise-policy-support-rotation@google.com"},
		Desc:     `A group of tests performing enrollment and will clobber the stateful partition.`,
	},
	{
		Name:     "essential-inputs",
		Contacts: []string{"essential-inputs-team@google.com"},
		Desc:     `A group of essential inputs IME and Virtual Keyboard tests.`,
	},
	{
		Name:     "firmware",
		Contacts: []string{"chromeos-engprod@google.com"},
		Desc:     `A group of tests for firmware (AP, EC, GSC)`,
		Subattrs: []*attr{
			{
				Name: "firmware_cr50",
				Desc: `Indicates that this is a test of the Google Security Chip firmware (Cr50).`,
			},
		},
	},
	{
		Name:     "flashrom",
		Contacts: []string{"chromeos-platform-syd@google.com"},
		Desc:     `A group of Flashrom destructive tests.`,
	},
	{
		Name:     "hwsec_destructive",
		Contacts: []string{"cros-hwsec@google.com"},
		Desc:     `A group of HWSec destructive tests that wipe and recreate encstateful in the tests.`,
	},
	{
		Name:     "runtime_probe",
		Contacts: []string{"chromeos-runtime-probe@google.com"},
		Desc: `A group of tests that tests the functionality of runtime probe.

The group of tests compare results probed by |runtime_probe| and corresponding information
in cros-labels decoded from HWID string.  These tests mainly check if runtime-probe works
as expected (in terms of D-Bus connection, probe function, and probe result).  For
short-term plan, only autotest can invoke these tests with host_info information.  That's
why we add this group and run tests of this group in a control file at
third_party/autotest/files/server/site_tests/tast/control.runtime-probe
`,
	},
	{
		Name:     "wificell",
		Contacts: []string{"chromeos-kernel-wifi@google.com"},
		Desc:     `The group of WiFi tests to be run with Wificell fixture.`,
		Subattrs: []*attr{
			{
				Name: "wificell_func",
				Desc: `Tests basic WiFi functionalities using Wificell fixture nightly.`,
			},
			{
				Name: "wificell_cq",
				Desc: `Similar to wificell_func, but triggered by CLs that touch specific code paths.`,
			},
			{
				Name: "wificell_unstable",
				Desc: `Indicates that this test is yet to be verified as stable.`,
			},
		},
	},
}

// validGroupMap is the name-keyed map of validGroups.
var validGroupMap = map[string]*group{}

func init() {
	// Initialize validGroupMap.
	for _, g := range validGroups {
		if _, ok := validGroupMap[g.Name]; ok {
			panic(fmt.Sprintf("Duplicated group definition %q found", g.Name))
		}
		validGroupMap[g.Name] = g
	}
}

const groupPrefix = "group:"

// checkKnownAttrs validate attrs against valid groups.
func checkKnownAttrs(attrs []string) error {
	const defPath = "chromiumos/tast/internal/testing/attr.go"

	var groups []*group
	for _, attr := range attrs {
		if isAutoAttr(attr) || !strings.HasPrefix(attr, groupPrefix) {
			continue
		}
		name := strings.TrimPrefix(attr, groupPrefix)
		g, ok := validGroupMap[name]
		if !ok {
			return fmt.Errorf("group %q is invalid; see %s for the full list of valid groups", name, defPath)
		}
		groups = append(groups, g)
	}

	for _, attr := range attrs {
		if isAutoAttr(attr) || strings.HasPrefix(attr, groupPrefix) {
			continue
		}
		// Allow the "disabled" attribute.
		// Note that manually-specified "disabled" attributes are checked on test instantiation
		// and prohibited. If we see the "disabled" attribute here, it is one internally added on
		// test instantiation for compatibility.
		// TODO(crbug.com/1005041): Remove this transitional handling.
		if attr == "disabled" {
			continue
		}
		found := false
	grouploop:
		for _, group := range groups {
			for _, subattr := range group.Subattrs {
				if attr == subattr.Name {
					found = true
					break grouploop
				}
			}
		}
		if !found {
			return fmt.Errorf("attribute %q is invalid in current groups; see %s for the full list of valid attributes", attr, defPath)
		}
	}

	return nil
}

// modifyAttrsForCompat modifies an attribute list for compatibility.
func modifyAttrsForCompat(attrs []string) []string {
	// If no "group:*" attribute is set, append the "disabled" attribute.
	// TODO(crbug.com/1005041): Remove this workaround once infra is updated to
	// not rely on the "disabled" attribute.
	hasGroup := false
	for _, a := range attrs {
		if strings.HasPrefix(a, groupPrefix) {
			hasGroup = true
			break
		}
	}

	if hasGroup {
		return attrs
	}
	return append(attrs, "disabled")
}
