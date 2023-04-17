// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"go.chromium.org/tast/core/tastuseonly/testing"
)

// Service contains information about a gRPC service exported for remote tests.
type Service = testing.Service

// ServiceState holds state relevant to a gRPC service.
type ServiceState = testing.ServiceState
