// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package telemetry is used to collect and write timing information about a process.
package telemetry_test

import (
	"context"
	"strings"
	"testing"

	"go.chromium.org/tast/core/internal/telemetry"
)

func TestSetPhase(t *testing.T) {
	ctx := context.Background()
	const testPhase = "Test Phase"
	const entityName = "tast"

	assertEmptyPhase(ctx, t)

	ctx = telemetry.SetPhase(ctx, testPhase, telemetry.Framework, entityName)
	if phase, ok := telemetry.GetPhase(ctx); !ok || !strings.Contains(phase.Name, testPhase) || !strings.Contains(phase.Name, entityName) {
		t.Errorf("Got phase %q; want %q", phase.Name, testPhase)
	}
}

func TestSetPhaseEndPrevious(t *testing.T) {
	ctx := context.Background()
	const testPhase1 = "Test Phase 1"
	const testPhase2 = "Test Phase 2"
	const entityName = "tast"

	assertEmptyPhase(ctx, t)

	ctx = telemetry.SetPhase(ctx, testPhase1, telemetry.Framework, entityName)
	if phase, ok := telemetry.GetPhase(ctx); !ok || !strings.Contains(phase.Name, testPhase1) || !strings.Contains(phase.Name, entityName) {
		t.Errorf("Got phase %q; want %q", phase.Name, testPhase1)
	}

	ctx = telemetry.SetPhase(ctx, testPhase2, telemetry.Framework, entityName)
	if phase, ok := telemetry.GetPhase(ctx); !ok || !strings.Contains(phase.Name, testPhase2) || !strings.Contains(phase.Name, entityName) {
		t.Errorf("Got phase %q; want %q", phase.Name, testPhase2)
	}
}

func TestSetPhaseBadNames(t *testing.T) {
	ctx := context.Background()
	const testPhase = "Test Phase"
	const entityName = "tast"

	assertEmptyPhase(ctx, t)

	ctx = telemetry.SetPhase(ctx, "", telemetry.Framework, entityName)
	assertEmptyPhase(ctx, t)

	ctx = telemetry.SetPhase(ctx, testPhase, telemetry.Framework, "")
	assertEmptyPhase(ctx, t)
}

func TestUnsetPhase(t *testing.T) {
	ctx := context.Background()
	const testPhase = "Test Phase"
	const entityName = "tast"

	assertEmptyPhase(ctx, t)

	ctx = telemetry.SetPhase(ctx, testPhase, telemetry.Framework, entityName)
	if phase, ok := telemetry.GetPhase(ctx); !ok || !strings.Contains(phase.Name, testPhase) || !strings.Contains(phase.Name, entityName) {
		t.Errorf("Got phase %q; want %q", phase.Name, testPhase)
	}

	ctx = telemetry.SetPhase(ctx, "", "", "")
	assertEmptyPhase(ctx, t)
}

func TestUnsetPhaseUnset(t *testing.T) {
	ctx := context.Background()
	assertEmptyPhase(ctx, t)
	ctx = telemetry.SetPhase(ctx, "", "", "")
	assertEmptyPhase(ctx, t)
}

func assertEmptyPhase(ctx context.Context, t *testing.T) {
	if phase, ok := telemetry.GetPhase(ctx); ok {
		t.Errorf("Got phase %q; want %q", phase.Name, "")
	}
}
