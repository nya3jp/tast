// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package jsonprotocol

import (
	"encoding/json"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.chromium.org/chromiumos/config/go/api"

	"chromiumos/tast/internal/planner"
	"chromiumos/tast/internal/protocol"
)

// BundleRunMode describes the bundle's behavior.
type BundleRunMode int

const (
	// BundleRunTestsMode indicates that the bundle should run all matched tests and write the results to stdout as
	// a sequence of JSON-marshaled control.Entity* control messages.
	BundleRunTestsMode BundleRunMode = 0
	// BundleListTestsMode indicates that the bundle should write information about matched tests to stdout as a
	// JSON array of testing.Test structs and exit.
	BundleListTestsMode BundleRunMode = 1
	// BundleRPCMode indicates that the bundle should run a gRPC server on the stdin/stdout.
	BundleRPCMode BundleRunMode = 2
	// BundleExportMetadataMode indicates that the bundle should compose and dump test metadata.
	BundleExportMetadataMode BundleRunMode = 3
	// BundleListFixturesMode indicates that the bundle should write information about all the fixtures
	// to stdout as a JSON array of testing.EntityInfo structs and exit.
	BundleListFixturesMode BundleRunMode = 4
)

// BundleArgs is used to pass arguments from test runners to test bundles.
// The runner executable writes the struct's JSON-marshaled representation to the bundle's stdin.
type BundleArgs struct {
	// Mode describes the mode that should be used by the bundle.
	Mode BundleRunMode `json:"mode"`

	// RunTests contains arguments used by BundleRunTestsMode.
	RunTests *BundleRunTestsArgs `json:"runTests,omitempty"`
	// ListTests contains arguments used by BundleListTestsMode.
	ListTests *BundleListTestsArgs `json:"listTests,omitempty"`
}

// FillDeprecated backfills deprecated fields from the corresponding non-deprecated fields.
// This method is called by test runners to ensure that args will be interpreted
// correctly by older test bundles.
func (a *BundleArgs) FillDeprecated() {
	// If there were any deprecated fields, we would fill them from the corresponding
	// non-deprecated fields here using command.CopyFieldIfNonZero for basic types or
	// manual copies for structs.
}

// PromoteDeprecated copies all non-zero-valued deprecated fields to the corresponding non-deprecated fields.
// Missing sub-structs (e.g. BundleRunTestsArgs and BundleListTestsArgs) are initialized.
// This method is called by test bundles to normalize args that were marshaled by an older test runner.
//
// If both an old and new field are set, the old field takes precedence. This is counter-intuitive but
// necessary: a default value for the new field may have been passed to run by Local or Remote. If the
// corresponding old field is non-zero, it was passed by an old runner (or by a new runner that called
// FillDeprecated), so we use the old field to make sure that it overrides the default.
func (a *BundleArgs) PromoteDeprecated() {
	// We don't have any deprecated fields right now.
}

// DeviceConfigJSON is a wrapper class for protocol.DeprecatedDeviceConfig to facilitate marshalling/unmarshalling.
type DeviceConfigJSON struct {
	// Proto contains the device configuration information about DUT.
	// Marshaling and unmarshaling of this field is handled in MarshalJSON/UnmarshalJSON
	// respectively.
	Proto *protocol.DeprecatedDeviceConfig `json:"-"`
}

// MarshalJSON marshals the protocol.DeprecatedDeviceConfig struct into JSON.
func (a *DeviceConfigJSON) MarshalJSON() ([]byte, error) {
	if a.Proto == nil {
		return []byte(`null`), nil
	}
	bin, err := proto.Marshal(a.Proto)
	if err != nil {
		return nil, err
	}
	return json.Marshal(bin)
}

// UnmarshalJSON unmarshals JSON blob for protocol.DeprecatedDeviceConfig.
func (a *DeviceConfigJSON) UnmarshalJSON(b []byte) error {
	var aux []byte
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	if len(aux) == 0 {
		return nil
	}
	var dc protocol.DeprecatedDeviceConfig
	if err := proto.Unmarshal(aux, &dc); err != nil {
		return err
	}
	a.Proto = &dc
	return nil
}

// HardwareFeaturesJSON is a wrapper class for api.HardwareFeatures to facilitate marshalling/unmarshalling.
type HardwareFeaturesJSON struct {
	// Proto contains the hardware info about DUT.
	// Marshaling and unmarshaling of this field is handled in MarshalJSON/UnmarshalJSON
	// respectively.
	Proto *api.HardwareFeatures `json:"-"`
}

// MarshalJSON marshals the api.HardwareFeatures struct into JSON.
func (a *HardwareFeaturesJSON) MarshalJSON() ([]byte, error) {
	if a.Proto == nil {
		return []byte(`null`), nil
	}
	bin, err := proto.Marshal(a.Proto)
	if err != nil {
		return nil, err
	}
	return json.Marshal(bin)
}

// UnmarshalJSON unmarshals JSON blob for api.HardwareFeatures.
func (a *HardwareFeaturesJSON) UnmarshalJSON(b []byte) error {
	var aux []byte
	if err := json.Unmarshal(b, &aux); err != nil || len(aux) == 0 {
		return err
	}
	var hw api.HardwareFeatures
	if err := proto.Unmarshal(aux, &hw); err != nil {
		return err
	}
	a.Proto = &hw
	return nil
}

// FeatureArgs includes all the feature related arguments.
type FeatureArgs struct {
	// TestVars contains names and values of runtime variables used to pass out-of-band data to tests.
	// Names correspond to testing.Test.Vars and values are accessed using testing.State.Var.
	TestVars map[string]string `json:"testVars,omitempty"`
	// MaybeMissingVars contains a regex compiled with regexp.Compile().
	// If every missing variable in testing.Test.VarDeps (exactly) matches the
	// regex, the test is skipped instead of failing.
	// If empty, no tests are skipped due to missing vars.
	MaybeMissingVars string `json:"maybeMissingVars,omitempty"`
	// CheckDeps indicates whether test runners should skip tests whose
	// dependencies are not satisfied by available features.
	CheckDeps bool `json:"checkSoftwareDeps,omitempty"`
	// AvailableSoftwareFeatures contains a list of software features supported by the DUT.
	AvailableSoftwareFeatures []string `json:"availableSoftwareFeatures,omitempty"`
	// UnavailableSoftwareFeatures contains a list of software features supported by the DUT.
	UnavailableSoftwareFeatures []string `json:"unavailableSoftwareFeatures,omitempty"`
	// DeviceConfig contains the hardware info about the DUT.
	// Marshaling and unmarshaling of this field is handled in MarshalJSON/UnmarshalJSON
	// respectively.
	DeviceConfig DeviceConfigJSON
	// HardwareFeatures contains the hardware info about DUT.
	// Marshaling and unmarshaling of this field is handled in MarshalJSON/UnmarshalJSON
	// respectively.
	HardwareFeatures HardwareFeaturesJSON
}

// Features returns protocol.Features to be used to check test dependencies.
func (a *FeatureArgs) Features() *protocol.Features {
	vars := make(map[string]string)
	for k, v := range a.TestVars {
		vars[k] = v
	}
	return &protocol.Features{
		CheckDeps: a.CheckDeps,
		Dut: &protocol.DUTFeatures{
			Software: &protocol.SoftwareFeatures{
				Available:   a.AvailableSoftwareFeatures,
				Unavailable: a.UnavailableSoftwareFeatures,
			},
			Hardware: &protocol.HardwareFeatures{
				DeprecatedDeviceConfig: a.DeviceConfig.Proto,
				HardwareFeatures:       a.HardwareFeatures.Proto,
			},
		},
		Infra: &protocol.InfraFeatures{
			Vars:             vars,
			MaybeMissingVars: a.MaybeMissingVars,
		},
	}
}

// BundleRunTestsArgs is nested within BundleArgs and contains arguments used by BundleRunTestsMode.
type BundleRunTestsArgs struct {
	// FeatureArgs includes all the feature related arguments.
	FeatureArgs

	// Patterns contains patterns (either empty to run all tests, exactly one attribute expression,
	// or one or more globs) describing which tests to run.
	Patterns []string `json:"patterns,omitempty"`

	// DataDir is the path to the directory containing test data files.
	DataDir string `json:"dataDir,omitempty"`
	// OutDir is the path to the base directory under which tests should write output files.
	OutDir string `json:"outDir,omitempty"`
	// TempDir is the path to the directory under which temporary files for tests are written.
	TempDir string `json:"tempDir,omitempty"`

	// Target is the DUT connection spec as [<user>@]host[:<port>].
	// It is only relevant for remote tests.
	Target string `json:"target,omitempty"`
	// KeyFile is the path to the SSH private key to use to connect to the DUT.
	// It is only relevant for remote tests.
	KeyFile string `json:"keyFile,omitempty"`
	// KeyDir is the directory containing SSH private keys (typically $HOME/.ssh).
	// It is only relevant for remote tests.
	KeyDir string `json:"keyDir,omitempty"`
	// TastPath contains the path to the tast binary that was executed to initiate testing.
	// It is only relevant for remote tests.
	TastPath string `json:"tastPath,omitempty"`
	// RunFlags contains a subset of the flags that were passed to the "tast run" command.
	// The included flags are ones that are necessary for core functionality,
	// e.g. paths to binaries used by the tast process and credentials for reconnecting to the DUT.
	// It is only relevant for remote tests.
	RunFlags []string `json:"runFlags,omitempty"`
	// LocalBundleDir is the directory on the DUT where local test bundle executables are located.
	// This path is used by remote tests to invoke gRPC services in local test bundles.
	// It is only relevant for remote tests.
	LocalBundleDir string `json:"localBundleDir,omitempty"`

	// Devservers contains URLs of devservers that can be used to download files.
	Devservers []string `json:"devservers,omitempty"`
	// TLWServer contains address of Test Lab Service Wiring APIs.
	TLWServer string `json:"tlwServer,omitempty"`
	// DUTName contains given DUT identifier to be passed to TLS Wiring API.
	DUTName string `json:"dutName,omitempty"`
	// companionDuts contains mapping between roles and addresses of companion DUTs.
	CompanionDUTs map[string]string `json:"companionDUTs,omitempty"`

	// BuildArtifactsURL is the URL of Google Cloud Storage directory, ending with a slash,
	// containing build artifacts for the current Chrome OS image.
	// If it is empty, DefaultBuildArtifactsURL in runner.Config is used.
	BuildArtifactsURL string `json:"buildArtifactsUrl,omitempty"`
	// DownloadMode specifies a strategy to download external data files.
	DownloadMode planner.DownloadMode `json:"downloadMode,omitempty"`

	// WaitUntilReady indicates that the test bundle's "ready" function (see ReadyFunc) should
	// be executed before any tests are executed.
	WaitUntilReady bool `json:"waitUntilReady,omitempty"`
	// HeartbeatInterval is the interval in seconds at which heartbeat messages are sent back
	// periodically from runners (before running bundles) and bundles. If this value is not
	// positive, heartbeat messages are not sent.
	HeartbeatInterval time.Duration `json:"heartbeatInterval,omitempty"`

	// SetUpErrors contains error messages happened on test setup (e.g. fixture SetUp). If its
	// length is non-zero, tests shouldn't run.
	SetUpErrors []string `json:"setUpErrors,omitempty"`

	// StartFixtureName is the remote fixture name that ran for the test.
	StartFixtureName string `json:"startFixtureName,omitempty"`
}

// Proto generates protocol.RunConfig.
func (a *BundleRunTestsArgs) Proto() (*protocol.RunConfig, *protocol.BundleConfig) {
	var bundleConfig *protocol.BundleConfig
	var tlwSelfName string
	// We consider that BundleRunTestsArgs is for remote tests if Target is
	// non-empty.
	if a.Target == "" {
		tlwSelfName = a.DUTName
	} else {
		companionDUTs := make(map[string]*protocol.DUTConfig)
		for role, addr := range a.CompanionDUTs {
			companionDUTs[role] = &protocol.DUTConfig{
				SshConfig: &protocol.SSHConfig{
					Target:  addr,
					KeyFile: a.KeyFile,
					KeyDir:  a.KeyDir,
				},
			}
		}
		bundleConfig = &protocol.BundleConfig{
			PrimaryTarget: &protocol.TargetDevice{
				DutConfig: &protocol.DUTConfig{
					SshConfig: &protocol.SSHConfig{
						Target:  a.Target,
						KeyFile: a.KeyFile,
						KeyDir:  a.KeyDir,
					},
					TlwName: a.DUTName,
				},
				BundleDir: a.LocalBundleDir,
			},
			CompanionDuts: companionDUTs,
			MetaTestConfig: &protocol.MetaTestConfig{
				TastPath: a.TastPath,
				RunFlags: a.RunFlags,
			},
		}
	}

	var startFixtErrors []*protocol.Error
	for _, r := range a.SetUpErrors {
		startFixtErrors = append(startFixtErrors, &protocol.Error{Reason: r})
	}

	return &protocol.RunConfig{
		Tests: a.Patterns,
		Dirs: &protocol.RunDirectories{
			DataDir: a.DataDir,
			OutDir:  a.OutDir,
			TempDir: a.TempDir,
		},
		Features: a.Features(),
		ServiceConfig: &protocol.ServiceConfig{
			Devservers:  a.Devservers,
			TlwServer:   a.TLWServer,
			TlwSelfName: tlwSelfName,
		},
		DataFileConfig: &protocol.DataFileConfig{
			DownloadMode:      a.DownloadMode.Proto(),
			BuildArtifactsUrl: a.BuildArtifactsURL,
		},
		StartFixtureState: &protocol.StartFixtureState{
			Name:   a.StartFixtureName,
			Errors: startFixtErrors,
		},
		HeartbeatInterval: ptypes.DurationProto(a.HeartbeatInterval),
		WaitUntilReady:    a.WaitUntilReady,
	}, bundleConfig
}

// BundleListTestsArgs is nested within BundleArgs and contains arguments used by BundleListTestsMode.
type BundleListTestsArgs struct {
	// FeatureArgs includes all the feature related arguments.
	FeatureArgs
	// Patterns contains patterns (either empty to list all tests, exactly one attribute expression,
	// or one or more globs) describing which tests to list.
	Patterns []string `json:"patterns,omitempty"`
}
