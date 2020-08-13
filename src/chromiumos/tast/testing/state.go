// Copyright 2017 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"chromiumos/tast/internal/testing"
)

// State holds state relevant to the execution of a single test.
//
// Parts of its interface are patterned after Go's testing.T type.
//
// State contains many pieces of data, and it's unclear which are actually being
// used when it's passed to a function. You should minimize the number of
// functions taking State as an argument. Instead you can pass State's derived
// values (e.g. s.DataPath("file.txt")) or ctx (to use with ContextLog or
// ContextOutDir etc.).
//
// It is intended to be safe when called concurrently by multiple goroutines
// while a test is running.
type State = testing.State

// PreState holds state relevant to the execution of a single precondition.
//
// This is a State for preconditions. See State's documentation for general
// guidance on how to treat PreState in preconditions.
type PreState = testing.PreState

// TestHookState holds state relevant to the execution of a test hook.
//
// This is a State for test hooks. See State's documentation for general
// guidance on how to treat TestHookState in test hooks.
type TestHookState = testing.TestHookState

// FixtState holds state relevant to the execution of a fixture.
//
// This is a State for fixtures. See State's documentation for general
// guidance on how to treat FixtState in fixtures.
type FixtState = testing.FixtState

// FixtTestState holds state relevant to the execution of test hooks in a fixture.
//
// This is a State for fixtures. See State's documentation for general
// guidance on how to treat FixtTestState in fixtures.
type FixtTestState = testing.FixtTestState

// Meta contains information about how the "tast" process used to initiate testing was run.
// It is used by remote tests in the "meta" category that run the tast executable to test Tast's behavior.
type Meta = testing.Meta

// RPCHint contains information needed to establish gRPC connections.
type RPCHint = testing.RPCHint

// CloudStorage allows Tast tests to read files on Google Cloud Storage.
type CloudStorage = testing.CloudStorage
