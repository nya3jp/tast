// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"chromiumos/tast/internal/testing"
)

// Service contains information about a gRPC service exported for remote tests.
type Service = testing.Service

// ServiceState holds state relevant to a gRPC service.
type ServiceState = testing.ServiceState
