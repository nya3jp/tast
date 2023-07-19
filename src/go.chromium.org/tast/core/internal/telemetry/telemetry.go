// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package telemetry is used to track test phase to provide telemetry metrics for the Tast tests.
package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast/core/internal/logging"
)

// PhaseInfo tracks metadata for the testing phase
type PhaseInfo struct {
	Name  string
	Start time.Time
}

type pKey int // unexported context.Context key type to avoid collisions with other packages

const phaseKey pKey = iota // key used for setting the phase to the context

var emptyPhase PhaseInfo = PhaseInfo{}

// entity types to mark phases
const (
	Test         string = "test"
	Framework    string = "framework"
	Fixture      string = "fixt"
	Precondition string = "pre"
)

// GetPhase pulls the Phase that is currently set on the Context.
//
// Returns true for the second value if the PhaseInfo provided is the currently
// set Phase.
// Returns false for the second value if the Phase is not currently set on the
// provided Context.
func GetPhase(ctx context.Context) (PhaseInfo, bool) {
	p, ok := ctx.Value(phaseKey).(PhaseInfo)
	if !ok || p == emptyPhase {
		return emptyPhase, false
	}
	return p, true
}

// SetPhase starts a new Phase on the provided Context. If a Phase is
// currently set on the Context, then the current Phase will be ended before
// the new one is started. To use SetPhase to unset the current Phase, pass
// in "" for any of the string arguments.
//
// phaseName: The name of the phase without entity information, e.g., "setup" for "tast setup"
// entityType: The type of entity being logged, e.g., telemetry.Framework for "tast setup"
// entityName: The name of the entity for this phase, e.g., "tast" for "tast setup"
//
// Returns a new Context with the new Phase set.
func SetPhase(ctx context.Context, phaseName, entityType, entityName string) context.Context {
	if phaseName == "" || entityName == "" {
		return unsetPhase(ctx)
	}

	if _, ok := GetPhase(ctx); ok {
		ctx = unsetPhase(ctx)
	}

	phaseName = generatePhaseName(entityType, entityName, phaseName)
	p := PhaseInfo{
		Name:  phaseName,
		Start: time.Now().UTC(),
	}
	ctx = context.WithValue(ctx, phaseKey, p)
	ctx = logging.SetLogPrefix(ctx, fmt.Sprintf("[%v] ", phaseName))

	return ctx
}

// unsetPhase ends the currently set Phase if one is set. This will not error if no Phase is set.
//
// Returns a new Context with no set Phase.
func unsetPhase(ctx context.Context) context.Context {
	phase, ok := GetPhase(ctx)
	if !ok {
		logging.Info(ctx, "Warning: No Phase currently set to end")
		return ctx
	}

	elapsed := time.Now().UTC().Sub(phase.Start)

	ctx = context.WithValue(ctx, phaseKey, emptyPhase)
	ctx = logging.UnsetLogPrefix(ctx)
	logging.Infof(ctx, "Phase \"%v\" ended after %v", phase.Name, elapsed)
	return ctx
}

func generatePhaseName(entityType, entityName, phaseName string) string {
	pn := entityName + " " + phaseName
	if entityType == Fixture || entityType == Precondition {
		pn = entityType + ":" + pn
	}
	return pn
}
