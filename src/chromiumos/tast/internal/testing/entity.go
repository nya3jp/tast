// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

// Entity contains metadata entities have in common.
type Entity struct {
	Data []string
}

// Entity constructs Entity from t.
func (t *TestInstance) Entity() *Entity {
	return &Entity{
		Data: append([]string(nil), t.Data...),
	}
}

// Entity constructs Entity from f.
func (f *Fixture) Entity() *Entity {
	return &Entity{
		Data: append([]string(nil), f.Data...),
	}
}
