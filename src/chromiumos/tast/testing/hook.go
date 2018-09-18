// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

// Hook is the type of a function to be run as a hook.
type Hook func(s *State)

// hookRegistry holds hook functions.
type hookRegistry struct {
	post []Hook // Hooks to be run after every test.
}

// newHookRegistry returns a new instance of hookRegistry.
func newHookRegistry() *hookRegistry {
	return &hookRegistry{}
}

// addPostHook registers a hook function to be run after every test.
func (r *hookRegistry) addPostHook(h Hook) {
	r.post = append(r.post, h)
}

// runPostHooks runs post-test hooks.
func (r *hookRegistry) runPostHooks(s *State) {
	for _, h := range r.post {
		h(s)
	}
}
