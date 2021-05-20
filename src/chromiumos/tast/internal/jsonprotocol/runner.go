// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package jsonprotocol

import (
	"encoding/json"
	"fmt"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/chromiumos/config/go/api"

	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/protocol"
)

// RunnerRunMode describes the runner's behavior.
type RunnerRunMode int

const (
	// RunnerRunTestsMode indicates that the runner should run all matched tests.
	RunnerRunTestsMode RunnerRunMode = 0
	// RunnerListTestsMode indicates that the runner should write information about matched tests to stdout as a
	// JSON array of testing.Test structs and exit.
	RunnerListTestsMode RunnerRunMode = 2
	// RunnerGetSysInfoStateMode indicates that the runner should write a JSON-marshaled RunnerGetSysInfoStateResult struct
	// to stdout and exit. It's used by the tast executable to get the initial state of the system before tests
	// are executed. This mode is only supported by local_test_runner.
	RunnerGetSysInfoStateMode RunnerRunMode = 3
	// RunnerCollectSysInfoMode indicates that the runner should collect system information that was written in the
	// course of testing and write a JSON-marshaled RunnerCollectSysInfoResult struct to stdout and exit. It's used by
	// the tast executable to get system info after testing is completed.
	// This mode is only supported by local_test_runner.
	RunnerCollectSysInfoMode RunnerRunMode = 4
	// RunnerGetDUTInfoMode indicates that the runner should return DUT information via a JSON-marshaled
	// RunnerGetDUTInfoResult struct written to stdout. This mode is only supported by local_test_runner.
	RunnerGetDUTInfoMode RunnerRunMode = 5
	// RunnerDownloadPrivateBundlesMode indicates that the runner should download private bundles from devservers,
	// install them to the DUT, write a JSON-marshaled RunnerDownloadPrivateBundlesResult struct to stdout and exit.
	// This mode is only supported by local_test_runner.
	RunnerDownloadPrivateBundlesMode RunnerRunMode = 6
	// RunnerListFixturesMode indicates that the runner should write information about fixtures to stdout
	// as a JSON serialized RunnerListFixturesResult.
	RunnerListFixturesMode RunnerRunMode = 7
	// RunnerRPCMode indicates that the runner should run a gRPC server on the stdin/stdout.
	RunnerRPCMode RunnerRunMode = 8
)

// RunnerArgs provides a backward- and forward-compatible way to pass arguments from the tast executable to test runners.
// The tast executable writes the struct's JSON-serialized representation to the runner's stdin.
type RunnerArgs struct {
	// Mode describes the mode that should be used by the runner.
	Mode RunnerRunMode `json:"mode"`

	// RunTests contains arguments used by RunnerRunTestsMode.
	RunTests *RunnerRunTestsArgs `json:"runTests,omitempty"`
	// ListTests contains arguments used by RunnerListTestsMode.
	ListTests *RunnerListTestsArgs `json:"listTests,omitempty"`
	// ListFixtures contains arguments used by RunnerListFixturesMode.
	ListFixtures *RunnerListFixturesArgs `json:"listFixtures,omitempty"`
	// CollectSysInfo contains arguments used by RunnerCollectSysInfoMode.
	CollectSysInfo *RunnerCollectSysInfoArgs `json:"collectSysInfo,omitempty"`
	// GetDUTInfo contains arguments used by RunnerGetDUTInfoMode.
	// Note that, for backward compatibility, the JSON's field name is getSoftwareFeatures.
	GetDUTInfo *RunnerGetDUTInfoArgs `json:"getSoftwareFeatures,omitempty"`
	// DownloadPrivateBundles contains arguments used by RunnerDownloadPrivateBundlesMode.
	DownloadPrivateBundles *RunnerDownloadPrivateBundlesArgs `json:"downloadPrivateBundles,omitempty"`

	// Report is set to true by readArgs if status should be reported via control messages rather
	// than human-readable log messages. This is true when args were supplied via stdin rather than
	// command-line flags, indicating that the runner was executed by the tast command. It's only relevant
	// for RunnerRunTestsMode.
	Report bool `json:"-"`
}

// BundleArgs creates a bundle.BundleArgs appropriate for running bundles in the supplied mode.
// The returned struct's slices should not be modified, as they are shared with a.
func (a *RunnerArgs) BundleArgs(mode BundleRunMode) (*BundleArgs, error) {
	ba := BundleArgs{Mode: mode}

	switch mode {
	case BundleRunTestsMode:
		switch a.Mode {
		case RunnerRunTestsMode:
			ba.RunTests = &a.RunTests.BundleArgs
		default:
			return nil, fmt.Errorf("can't make RunTests bundle args in runner mode %d", int(a.Mode))
		}
	case BundleListTestsMode:
		switch a.Mode {
		case RunnerRunTestsMode:
			// We didn't receive ListTests args, so copy the shared patterns field from RunTests.
			ba.ListTests = &BundleListTestsArgs{Patterns: a.RunTests.BundleArgs.Patterns}
		case RunnerListTestsMode:
			ba.ListTests = &a.ListTests.BundleArgs
		default:
			return nil, fmt.Errorf("can't make ListTests bundle args in runner mode %d", int(a.Mode))
		}
	}

	// Backfill deprecated fields in case we're executing an old test bundle.
	ba.FillDeprecated()

	return &ba, nil
}

// FillDeprecated backfills deprecated fields from the corresponding non-deprecated fields.
// This method is called by the tast process to ensure that args will be interpreted
// correctly by older test runners.
func (a *RunnerArgs) FillDeprecated() {
	// If there were any deprecated fields, we would fill them from the corresponding
	// non-deprecated fields here using command.CopyFieldIfNonZero for basic types or
	// manual copies for structs.
	if a.RunTests != nil {
		command.CopyFieldIfNonZero(&a.RunTests.BundleArgs.BuildArtifactsURL, &a.RunTests.BuildArtifactsURLDeprecated)
	}
}

// PromoteDeprecated copies all non-zero-valued deprecated fields to the corresponding non-deprecated fields.
// Missing sub-structs (e.g. RunnerRunTestsArgs and RunnerListTestsArgs) are initialized.
// This method is called by test runners to normalize args that were marshaled by an older tast executable.
//
// If both an old and new field are set, the old field takes precedence. This is counter-intuitive but
// necessary: a default value for the new field may have been passed to Run. If the corresponding old field
// is non-zero, it was passed by an old tast executable (or by a new executable that called FillDeprecated),
// so we use the old field to make sure that it overrides the default.
func (a *RunnerArgs) PromoteDeprecated() {
	if a.RunTests != nil {
		command.CopyFieldIfNonZero(&a.RunTests.BuildArtifactsURLDeprecated, &a.RunTests.BundleArgs.BuildArtifactsURL)
	}
}

// RunnerRunTestsArgs is nested within RunnerArgs and contains arguments used by RunnerRunTestsMode.
type RunnerRunTestsArgs struct {
	// BundleArgs contains arguments that are relevant to test bundles.
	BundleArgs BundleRunTestsArgs `json:"bundleArgs"`
	// BundleGlob is a glob-style path matching test bundles to execute.
	BundleGlob string `json:"bundleGlob,omitempty"`
	// Devservers contains URLs of devservers that can be used to download files.
	Devservers []string `json:"devservers,omitempty"`
	// BuildArtifactsURLDeprecated is the URL of Google Cloud Storage directory, ending with a slash,
	// containing build artifacts for the current Chrome OS image.
	// If it is empty, DefaultBuildArtifactsURL in runner.Config is used.
	// DEPRECATED: Use bundle.BundleRunTestsArgs.BuildArtifactsURL instead.
	BuildArtifactsURLDeprecated string `json:"buildArtifactsUrl,omitempty"`
}

// RunnerListTestsArgs is nested within RunnerArgs and contains arguments used by RunnerListTestsMode.
type RunnerListTestsArgs struct {
	// BundleArgs contains arguments that are relevant to test bundles.
	BundleArgs BundleListTestsArgs `json:"bundleArgs"`
	// BundleGlob is a glob-style path matching test bundles to execute.
	BundleGlob string `json:"bundleGlob,omitempty"`
}

// RunnerListTestsResult holds the result of a ListTestsMode command
type RunnerListTestsResult []*EntityWithRunnabilityInfo

// RunnerListFixturesArgs is nested within RunnerArgs and contains arguments used by RunnerListFixturesMode.
type RunnerListFixturesArgs struct {
	// BundleGlob is a glob-style path matching test bundles to execute.
	BundleGlob string `json:"bundleGlob,omitempty"`
}

// RunnerListFixturesResult holds the result of a RunnerListFixturesMode command.
type RunnerListFixturesResult struct {
	// Fixtures maps bundle path to the fixtures it contains.
	Fixtures map[string][]*EntityInfo `json:"fixtures,omitempty"`
}

// RunnerGetSysInfoStateResult holds the result of a RunnerGetSysInfoStateMode command.
type RunnerGetSysInfoStateResult struct {
	// SysInfoState contains the collected state.
	State SysInfoState `json:"state"`
	// Warnings contains descriptions of non-fatal errors encountered while collecting data.
	// Each warning can be logged directly without additional information.
	Warnings []string `json:"warnings,omitempty"`
}

// RunnerCollectSysInfoArgs is nested within RunnerArgs and holds arguments used by RunnerCollectSysInfoMode.
type RunnerCollectSysInfoArgs struct {
	// InitialState describes the pre-testing state of the DUT. It should be generated by a RunnerGetSysInfoStateMode
	// command executed before tests are run.
	InitialState SysInfoState `json:"initialState"`
}

// RunnerCollectSysInfoResult contains the result of a RunnerCollectSysInfoMode command.
type RunnerCollectSysInfoResult struct {
	// LogDir is the directory where log files were copied. The caller should delete it.
	LogDir string `json:"logDir,omitempty"`
	// CrashDir is the directory where minidump crash files were copied. The caller should delete it.
	CrashDir string `json:"crashDir,omitempty"`
	// Warnings contains descriptions of non-fatal errors encountered while collecting data.
	Warnings []string `json:"warnings,omitempty"`
}

// RunnerGetDUTInfoArgs is nested within RunnerArgs and contains arguments used by RunnerGetDUTInfoMode.
type RunnerGetDUTInfoArgs struct {
	// ExtraUSEFlags lists USE flags that should be treated as being set an addition to
	// the ones read from Config.USEFlagsFile when computing the feature sets for RunnerGetDUTInfoResult.
	ExtraUSEFlags []string `json:"extraUseFlags,omitempty"`

	// RequestDeviceConfig specifies if RunnerGetDUTInfoMode should return a device.Config instance
	// generated from runtime DUT configuration.
	RequestDeviceConfig bool `json:"requestDeviceConfig,omitempty"`
}

// RunnerGetDUTInfoResult contains the result of a RunnerGetDUTInfoMode command.
type RunnerGetDUTInfoResult struct {
	// SoftwareFeatures contains the information about the software features of the DUT.
	// For backward compatibility, in JSON format, fields are flatten.
	// This struct has MarshalJSON/UnmarshalJSON and the serialization/deserialization
	// of this field are handled in the methods respectively.
	SoftwareFeatures *protocol.SoftwareFeatures `json:"-"`

	// DeviceConfig contains the DUT's device charactersitic that is not covered by HardwareFeatures.
	// Similar to SoftwareFeatures field, the serialization/deserialization
	// of this field are handled in MarshalJSON/UnmarshalJSON respectively.
	DeviceConfig *protocol.DeprecatedDeviceConfig `json:"-"`

	// HardwareFeatures contains the DUT's device characteristic.
	// Similar to SoftwareFeatures field, the serialization/deserialization
	// of this field are handled in MarshalJSON/UnmarshalJSON respectively.
	HardwareFeatures *api.HardwareFeatures `json:"-"`

	// OSVersion contains the DUT's OS Version
	OSVersion string `json:"osVersion,omitempty"`

	// DefaultBuildArtifactsURL specified the default URL of the build artifacts.
	DefaultBuildArtifactsURL string `json:"defaultBuildArtifactsURL,omitempty"`

	// Warnings contains descriptions of non-fatal errors encountered while determining features.
	Warnings []string `json:"warnings,omitempty"`
}

// MarshalJSON marshals the given RunnerGetDUTInfoResult with handing protocol
// backward compatibility.
func (r *RunnerGetDUTInfoResult) MarshalJSON() ([]byte, error) {
	var available, missing []string
	if r.SoftwareFeatures != nil {
		available = r.SoftwareFeatures.GetAvailable()
		missing = r.SoftwareFeatures.GetUnavailable()
	}

	var dc []byte
	if r.DeviceConfig != nil {
		var err error
		dc, err = proto.Marshal(r.DeviceConfig)
		if err != nil {
			return nil, err
		}
	}
	var hf []byte
	if r.HardwareFeatures != nil {
		var err error
		hf, err = proto.Marshal(r.HardwareFeatures)
		if err != nil {
			return nil, err
		}
	}

	type Alias RunnerGetDUTInfoResult
	return json.Marshal(struct {
		Available        []string `json:"available,omitempty"`
		Missing          []string `json:"missing,omitempty"`
		DeviceConfig     []byte   `json:"deviceConfig,omitempty"`
		HardwareFeatures []byte   `json:"hardwareFeatures,omitempty"`
		*Alias
	}{
		Available:        available,
		Missing:          missing,
		DeviceConfig:     dc,
		HardwareFeatures: hf,
		Alias:            (*Alias)(r),
	})
}

// UnmarshalJSON unmarshals the given b to this r object with handing protocol
// backward compatibility.
func (r *RunnerGetDUTInfoResult) UnmarshalJSON(b []byte) error {
	type Alias RunnerGetDUTInfoResult
	aux := struct {
		Available        []string `json:"available,omitempty"`
		Missing          []string `json:"missing,omitempty"`
		DeviceConfig     []byte   `json:"deviceConfig,omitempty"`
		HardwareFeatures []byte   `json:"hardwareFeatures,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	if len(aux.Available) > 0 || len(aux.Missing) > 0 {
		r.SoftwareFeatures = &protocol.SoftwareFeatures{
			Available:   aux.Available,
			Unavailable: aux.Missing,
		}
	}
	if len(aux.DeviceConfig) > 0 {
		var dc protocol.DeprecatedDeviceConfig
		if err := proto.Unmarshal(aux.DeviceConfig, &dc); err != nil {
			return err
		}
		r.DeviceConfig = &dc
	}
	if len(aux.HardwareFeatures) > 0 {
		var hf api.HardwareFeatures
		if err := proto.Unmarshal(aux.HardwareFeatures, &hf); err != nil {
			return err
		}
		r.HardwareFeatures = &hf
	}
	return nil
}

// Proto generates protocol.DUTInfo.
func (r *RunnerGetDUTInfoResult) Proto() *protocol.DUTInfo {
	return &protocol.DUTInfo{
		Features: &protocol.DUTFeatures{
			Software: r.SoftwareFeatures,
			Hardware: &protocol.HardwareFeatures{
				HardwareFeatures:       r.HardwareFeatures,
				DeprecatedDeviceConfig: r.DeviceConfig,
			},
		},
		OsVersion:                r.OSVersion,
		DefaultBuildArtifactsUrl: r.DefaultBuildArtifactsURL,
	}
}

// SysInfoState contains the state of the DUT's system information.
type SysInfoState struct {
	// LogInodeSizes maps from each log file's inode to its size in bytes.
	LogInodeSizes map[uint64]int64 `json:"logInodeSizes,omitempty"`
	// UnifiedLogCursor contains an opaque cursor pointing at the current tip of unified system logs.
	// The name of json field is "journaldCursor" for historical reason.
	UnifiedLogCursor string `json:"journaldCursor,omitempty"`
	// MinidumpPaths contains absolute paths to minidump crash files.
	MinidumpPaths []string `json:"minidumpPaths,omitempty"`
}

// RunnerDownloadPrivateBundlesArgs is nested within RunnerArgs and contains arguments used by RunnerDownloadPrivateBundlesMode.
type RunnerDownloadPrivateBundlesArgs struct {
	// Devservers contains URLs of devservers that can be used to download files.
	Devservers []string `json:"devservers,omitempty"`

	// TLWServer contains host and port name of TLW server that can be used for downloading files.
	// When this is set, it takes precedence over Devservers.
	TLWServer string `json:"tlsServer,omitempty"`

	// DUTName contains the name of the DUT recognized by the TLW service.
	// This must be set when TLWServer is used.
	DUTName string `json:"dutName,omitempty"`

	// BuildArtifactsURL is the URL of Google Cloud Storage directory, ending with a slash,
	// containing build artifacts for the current Chrome OS image.
	// If it is empty, DefaultBuildArtifactsURL in runner.Config is used.
	BuildArtifactsURL string `json:"buildArtifactsUrl,omitempty"`
}

// RunnerDownloadPrivateBundlesResult contains the result of a RunnerDownloadPrivateBundlesMode command.
type RunnerDownloadPrivateBundlesResult struct {
	// Messages contains log messages emitted while downloading test bundles.
	Messages []string `json:"logs,omitempty"`
}
