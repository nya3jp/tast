// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package jsonprotocol

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"

	"chromiumos/tast/errors"
	"chromiumos/tast/internal/protocol"
)

// EntityInfoFromProto converts protocol.Entity to jsonprotocol.EntityInfo.
func EntityInfoFromProto(e *protocol.Entity) (*EntityInfo, error) {
	var tp EntityType
	switch e.GetType() {
	case protocol.EntityType_TEST:
		tp = EntityTest
	case protocol.EntityType_FIXTURE:
		tp = EntityFixture
	default:
		return nil, errors.Errorf("unknown entity type: %v", e.GetType())
	}

	var timeout time.Duration
	if topb := e.GetLegacyData().GetTimeout(); topb != nil {
		to, err := ptypes.Duration(topb)
		if err != nil {
			return nil, errors.Wrap(err, "cannot convert timeout")
		}
		timeout = to
	}

	return &EntityInfo{
		Type:         tp,
		Name:         e.GetName(),
		Pkg:          e.GetPackage(),
		Desc:         e.GetDescription(),
		Contacts:     append([]string(nil), e.GetContacts().GetEmails()...),
		Attr:         append([]string(nil), e.GetAttributes()...),
		Data:         append([]string(nil), e.GetDependencies().GetDataFiles()...),
		Vars:         append([]string(nil), e.GetLegacyData().GetVariables()...),
		VarDeps:      append([]string(nil), e.GetLegacyData().GetVariableDeps()...),
		SoftwareDeps: append([]string(nil), e.GetLegacyData().GetSoftwareDeps()...),
		ServiceDeps:  append([]string(nil), e.GetDependencies().GetServices()...),
		Fixture:      e.GetFixture(),
		Timeout:      timeout,
		Bundle:       e.GetLegacyData().GetBundle(),
	}, nil
}

// MustEntityInfoFromProto is similar to EntityInfoFromProto, but it panics
// when it fails to convert.
func MustEntityInfoFromProto(e *protocol.Entity) *EntityInfo {
	ei, err := EntityInfoFromProto(e)
	if err != nil {
		panic(fmt.Sprintf("MustEntityInfoFromProto: %v", err))
	}
	return ei
}

// ErrorFromProto converts protocol.Error to jsonprotocol.Error.
func ErrorFromProto(e *protocol.Error) *Error {
	return &Error{
		Reason: e.GetReason(),
		File:   e.GetLocation().GetFile(),
		Line:   int(e.GetLocation().GetLine()),
		Stack:  e.GetLocation().GetStack(),
	}
}

// EntityWithRunnabilityInfoFromProto converts protocol.ResolvedEntity to
// jsonprotocol.EntityWithRunnabilityInfo.
func EntityWithRunnabilityInfoFromProto(e *protocol.ResolvedEntity) (*EntityWithRunnabilityInfo, error) {
	ei, err := EntityInfoFromProto(e.GetEntity())
	if err != nil {
		return nil, err
	}
	return &EntityWithRunnabilityInfo{
		EntityInfo: *ei,
		SkipReason: strings.Join(e.GetSkip().GetReasons(), "; "),
	}, nil
}

// MustEntityWithRunnabilityInfoFromProto is similar to
// EntityWithRunnabilityInfoFromProto, but it panics when it fails to convert.
func MustEntityWithRunnabilityInfoFromProto(e *protocol.ResolvedEntity) *EntityWithRunnabilityInfo {
	ei, err := EntityWithRunnabilityInfoFromProto(e)
	if err != nil {
		panic(fmt.Sprintf("MustEntityWithRunnabilityInfoFromProto: %v", err))
	}
	return ei
}
