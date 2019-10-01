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
			// TODO(crbug.com/1005041): Consider removing this attribute. After introducing group:mainline,
			// we can disable a test by removing group:mainline.
			{
				Name: "disabled",
				Desc: `Indicates that this test should not be run in the lab.

Usually faulty tests should be marked informational, instead of disabled, so
that their results can be tracked without blocking changes. However we might
want to disable tests in some cases, for example when they interact with other
tests badly.
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
				Desc: `Indicate this test is focos on video encode/decode.`,
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
		},
	},
	{
		Name:     "stress",
		Contacts: []string{"chromeos-engprod@google.com"},
		Desc:     `A group of stress tests.`,
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

// checkKnownAttrs validate attrs against valid groups.
func checkKnownAttrs(attrs []string) error {
	const (
		groupPrefix = "group:"
		defPath     = "chromiumos/tast/testing/attr.go"
	)

	var groups []*group
	for _, attr := range attrs {
		if !strings.HasPrefix(attr, groupPrefix) {
			continue
		}
		name := strings.TrimPrefix(attr, groupPrefix)
		g, ok := validGroupMap[name]
		if !ok {
			return fmt.Errorf("group %q is invalid; see %s for the full list of valid groups", name, defPath)
		}
		groups = append(groups, g)
	}

	// For transition, treat tests belonging to no group as mainline tests.
	// TODO(crbug.com/1005041): Remove this transitional hack.
	if len(groups) == 0 {
		groups = append(groups, validGroupMap["mainline"])
	}

	for _, attr := range attrs {
		if strings.HasPrefix(attr, groupPrefix) {
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
