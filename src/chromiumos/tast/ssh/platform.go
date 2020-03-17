// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ssh

import "chromiumos/tast/host"

// Platform defines platform-specific behaviours for SSH connections.
type Platform = host.Platform

// DefaultPlatform represents a system with a generic POSIX shell.
var DefaultPlatform = host.DefaultPlatform
