// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package protocol

import (
	"go.chromium.org/tast/core/internal/logging"
)

// LevelToProto converts a logging.Level to the protocol.LogLevel.
func LevelToProto(level logging.Level) LogLevel {
	switch level {
	case logging.LevelInfo:
		return LogLevel_INFO
	case logging.LevelDebug:
		return LogLevel_DEBUG
	default:
		return LogLevel_LOGLEVEL_UNSPECIFIED
	}
}

// ProtoToLevel converts a protocol.LogLevel to the logging.Level.
func ProtoToLevel(level LogLevel) logging.Level {
	switch level {
	case LogLevel_INFO:
		return logging.LevelInfo
	case LogLevel_DEBUG:
		return logging.LevelDebug
	default:
		// For LogLevel_LOGLEVEL_UNSPECIFIED, assume that it is coming from version skew, and default to info.
		return logging.LevelInfo
	}
}
