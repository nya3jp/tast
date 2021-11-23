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
	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/protocol"
)

// EntityTypeFromProto converts protocol.EntityType to jsonprotocol.EntityType.
func EntityTypeFromProto(t protocol.EntityType) (EntityType, error) {
	switch t {
	case protocol.EntityType_TEST:
		return EntityTest, nil
	case protocol.EntityType_FIXTURE:
		return EntityFixture, nil
	default:
		return EntityTest, errors.Errorf("unknown entity type: %v", t)
	}
}

// EntityInfoFromProto converts protocol.Entity to jsonprotocol.EntityInfo.
func EntityInfoFromProto(e *protocol.Entity) (*EntityInfo, error) {
	tp, err := EntityTypeFromProto(e.GetType())
	if err != nil {
		return nil, err
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

// RunnerGetDUTInfoResultFromProto converts protocol.GetDUTInfoResponse to
// jsonprotocol.RunnerGetDUTInfoResult.
func RunnerGetDUTInfoResultFromProto(res *protocol.GetDUTInfoResponse) *RunnerGetDUTInfoResult {
	info := res.GetDutInfo()
	return &RunnerGetDUTInfoResult{
		SoftwareFeatures:         info.GetFeatures().GetSoftware(),
		DeviceConfig:             info.GetFeatures().GetHardware().GetDeprecatedDeviceConfig(),
		HardwareFeatures:         info.GetFeatures().GetHardware().GetHardwareFeatures(),
		OSVersion:                info.GetOsVersion(),
		DefaultBuildArtifactsURL: info.GetDefaultBuildArtifactsUrl(),
	}
}

// RunnerGetSysInfoStateResultFromProto converts protocol.GetSysInfoStateResponse to
// jsonprotocol.RunnerGetSysInfoStateResult.
func RunnerGetSysInfoStateResultFromProto(res *protocol.GetSysInfoStateResponse) *RunnerGetSysInfoStateResult {
	return &RunnerGetSysInfoStateResult{
		State: *SysInfoStateFromProto(res.GetState()),
	}
}

// RunnerCollectSysInfoResultFromProto converts protocol.CollectSysInfoResponse to
// jsonprotocol.RunnerCollectSysInfoResult.
func RunnerCollectSysInfoResultFromProto(res *protocol.CollectSysInfoResponse) *RunnerCollectSysInfoResult {
	return &RunnerCollectSysInfoResult{
		LogDir:   res.GetLogDir(),
		CrashDir: res.GetCrashDir(),
	}
}

// RunnerDownloadPrivateBundlesResultFromProto converts protocol.DownloadPrivateBundlesResponse to
// jsonprotocol.RunnerDownloadPrivateBundlesResult.
func RunnerDownloadPrivateBundlesResultFromProto(res *protocol.DownloadPrivateBundlesResponse) *RunnerDownloadPrivateBundlesResult {
	return &RunnerDownloadPrivateBundlesResult{}
}

// SysInfoStateFromProto converts protocol.SysInfoState to
// jsonprotocol.SysInfoState.
func SysInfoStateFromProto(state *protocol.SysInfoState) *SysInfoState {
	return &SysInfoState{
		LogInodeSizes:    state.GetLogInodeSizes(),
		UnifiedLogCursor: state.GetUnifiedLogCursor(),
		MinidumpPaths:    state.GetMinidumpPaths(),
	}
}

// FeatureArgsFromProto converts protocol.Features to jsonprotocol.FeatureArgs.
func FeatureArgsFromProto(f *protocol.Features) *FeatureArgs {
	return &FeatureArgs{
		TestVars:                    f.GetInfra().GetVars(),
		MaybeMissingVars:            f.GetInfra().GetMaybeMissingVars(),
		CheckDeps:                   f.GetCheckDeps(),
		AvailableSoftwareFeatures:   f.GetDut().GetSoftware().GetAvailable(),
		UnavailableSoftwareFeatures: f.GetDut().GetSoftware().GetUnavailable(),
		DeviceConfig:                DeviceConfigJSON{Proto: f.GetDut().GetHardware().GetDeprecatedDeviceConfig()},
		HardwareFeatures:            HardwareFeaturesJSON{Proto: f.GetDut().GetHardware().GetHardwareFeatures()},
	}
}

// BundleRunTestsArgsFromProto creates jsonprotocol.BundleRunTestsArgs from
// protocol.BundleConfig and protocol.RunConfig.
func BundleRunTestsArgsFromProto(bcfg *protocol.BundleConfig, rcfg *protocol.RunConfig) (*BundleRunTestsArgs, error) {
	downloadMode, err := planner.DownloadModeFromProto(rcfg.GetDataFileConfig().GetDownloadMode())
	if err != nil {
		return nil, err
	}
	heartbeatInterval, err := ptypes.Duration(rcfg.GetHeartbeatInterval())
	if err != nil {
		return nil, err
	}
	companionDUTs := make(map[string]string)
	for name, dutCfg := range bcfg.GetCompanionDuts() {
		companionDUTs[name] = dutCfg.GetSshConfig().GetConnectionSpec()
	}
	var setupErrors []string
	for _, e := range rcfg.GetStartFixtureState().GetErrors() {
		setupErrors = append(setupErrors, e.GetReason())
	}
	return &BundleRunTestsArgs{
		FeatureArgs:       *FeatureArgsFromProto(rcfg.GetFeatures()),
		Patterns:          rcfg.GetTests(),
		DataDir:           rcfg.GetDirs().GetDataDir(),
		OutDir:            rcfg.GetDirs().GetOutDir(),
		TempDir:           rcfg.GetDirs().GetTempDir(),
		ConnectionSpec:    bcfg.GetPrimaryTarget().GetDutConfig().GetSshConfig().GetConnectionSpec(),
		KeyFile:           bcfg.GetPrimaryTarget().GetDutConfig().GetSshConfig().GetKeyFile(),
		KeyDir:            bcfg.GetPrimaryTarget().GetDutConfig().GetSshConfig().GetKeyDir(),
		TastPath:          bcfg.GetMetaTestConfig().GetTastPath(),
		RunFlags:          bcfg.GetMetaTestConfig().GetRunFlags(),
		LocalBundleDir:    bcfg.GetPrimaryTarget().GetBundleDir(),
		Devservers:        rcfg.GetServiceConfig().GetDevservers(),
		DUTServer:         rcfg.GetServiceConfig().GetDutServer(),
		TLWServer:         rcfg.GetServiceConfig().GetTlwServer(),
		DUTName:           rcfg.GetServiceConfig().GetTlwSelfName(),
		CompanionDUTs:     companionDUTs,
		BuildArtifactsURL: rcfg.GetDataFileConfig().GetBuildArtifactsUrl(),
		DownloadMode:      downloadMode,
		WaitUntilReady:    rcfg.GetWaitUntilReady(),
		HeartbeatInterval: heartbeatInterval,
		SetUpErrors:       setupErrors,
		StartFixtureName:  rcfg.GetStartFixtureState().GetName(),
	}, nil
}
