// Copyright 2018 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bundle

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/chromiumos/config/go/api"
	"go.chromium.org/chromiumos/infra/proto/go/device"

	"chromiumos/tast/internal/command"
	"chromiumos/tast/internal/dep"
	"chromiumos/tast/internal/planner"
)

// RunMode describes the bundle's behavior.
type RunMode int

const (
	// RunTestsMode indicates that the bundle should run all matched tests and write the results to stdout as
	// a sequence of JSON-marshaled control.Entity* control messages.
	RunTestsMode RunMode = 0
	// ListTestsMode indicates that the bundle should write information about matched tests to stdout as a
	// JSON array of testing.Test structs and exit.
	ListTestsMode = 1
	// RPCMode indicates that the bundle should run a gRPC server on the stdin/stdout.
	RPCMode = 2
	// ExportMetadataMode indicates that the bundle should compose and dump test metadata.
	ExportMetadataMode = 3
	// ListFixturesMode indicates that the bundle should write information about all the fixtures
	// to stdout as a JSON array of testing.FixtureInfo structs and exit.
	ListFixturesMode = 4
)

// Args is used to pass arguments from test runners to test bundles.
// The runner executable writes the struct's JSON-marshaled representation to the bundle's stdin.
type Args struct {
	// Mode describes the mode that should be used by the bundle.
	Mode RunMode `json:"mode"`

	// RunTests contains arguments used by RunTestsMode.
	RunTests *RunTestsArgs `json:"runTests,omitempty"`
	// ListTests contains arguments used by ListTestsMode.
	ListTests *ListTestsArgs `json:"listTests,omitempty"`
}

// FillDeprecated backfills deprecated fields from the corresponding non-deprecated fields.
// This method is called by test runners to ensure that args will be interpreted
// correctly by older test bundles.
func (a *Args) FillDeprecated() {
	// If there were any deprecated fields, we would fill them from the corresponding
	// non-deprecated fields here using command.CopyFieldIfNonZero for basic types or
	// manual copies for structs.
}

// PromoteDeprecated copies all non-zero-valued deprecated fields to the corresponding non-deprecated fields.
// Missing sub-structs (e.g. RunTestsArgs and ListTestsArgs) are initialized.
// This method is called by test bundles to normalize args that were marshaled by an older test runner.
//
// If both an old and new field are set, the old field takes precedence. This is counter-intuitive but
// necessary: a default value for the new field may have been passed to run by Local or Remote. If the
// corresponding old field is non-zero, it was passed by an old runner (or by a new runner that called
// FillDeprecated), so we use the old field to make sure that it overrides the default.
func (a *Args) PromoteDeprecated() {
	// We don't have any deprecated fields right now.
}

// RunTestsArgs is nested within Args and contains arguments used by RunTestsMode.
type RunTestsArgs struct {
	// Patterns contains patterns (either empty to run all tests, exactly one attribute expression,
	// or one or more globs) describing which tests to run.
	Patterns []string `json:"patterns,omitempty"`

	// TestVars contains names and values of runtime variables used to pass out-of-band data to tests.
	// Names correspond to testing.Test.Vars and values are accessed using testing.State.Var.
	TestVars map[string]string `json:"testVars,omitempty"`

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

	// CheckSoftwareDeps is true if each test's SoftwareDeps field should be checked against
	// AvailableSoftwareFeatures and UnavailableSoftwareFeatures.
	CheckSoftwareDeps bool `json:"checkSoftwareDeps,omitempty"`
	// AvailableSoftwareFeatures contains a list of software features supported by the DUT.
	AvailableSoftwareFeatures []string `json:"availableSoftwareFeatures,omitempty"`
	// UnavailableSoftwareFeatures contains a list of software features supported by the DUT.
	UnavailableSoftwareFeatures []string `json:"unavailableSoftwareFeatures,omitempty"`
	// DeviceConfig contains the hardware info about the DUT.
	// Marshaling and unmarshaling of this field is handled in MarshalJSON/UnmarshalJSON
	// respectively.
	// Deprecated. Use HardwareFeatures instead.
	DeviceConfig *device.Config `json:"-"`
	// HardwareFeatures contains the hardware info about DUT.
	// Marshaling and unmarshaling of this field is handled in MarshalJSON/UnmarshalJSON
	// respectively.
	HardwareFeatures *api.HardwareFeatures `json:"-"`

	// Devservers contains URLs of devservers that can be used to download files.
	Devservers []string `json:"devservers,omitempty"`
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
}

// Features returns dep.Features to be used to check test dependencies.
func (a *RunTestsArgs) Features() *dep.Features {
	var f dep.Features
	if a.CheckSoftwareDeps {
		f.Software = &dep.SoftwareFeatures{
			Available:   a.AvailableSoftwareFeatures,
			Unavailable: a.UnavailableSoftwareFeatures,
		}
		f.Hardware = &dep.HardwareFeatures{
			DC:       a.DeviceConfig,
			Features: a.HardwareFeatures,
		}
	}
	return &f
}

// MarshalJSON marshals the RunTestsArgs struct into JSON.
func (a *RunTestsArgs) MarshalJSON() ([]byte, error) {
	var dc []byte
	if a.DeviceConfig != nil {
		var err error
		dc, err = proto.Marshal(a.DeviceConfig)
		if err != nil {
			return nil, err
		}
	}
	var features []byte
	if a.HardwareFeatures != nil {
		var err error
		features, err = proto.Marshal(a.HardwareFeatures)
		if err != nil {
			return nil, err
		}
	}

	// Use alias to break the infinite recursion in json.Marshal.
	type Alias RunTestsArgs
	return json.Marshal(struct {
		DeviceConfig     []byte `json:"deviceConfig"`
		HardwareFeatures []byte `json:"hardwareFeatures"`
		*Alias
	}{
		DeviceConfig:     dc,
		HardwareFeatures: features,
		Alias:            (*Alias)(a),
	})
}

// UnmarshalJSON unmarshals JSON blob.
func (a *RunTestsArgs) UnmarshalJSON(b []byte) error {
	// Use alias to break the infinite recursion in json.Unmarshal.
	type Alias RunTestsArgs
	aux := struct {
		DeviceConfig     []byte `json:"deviceConfig"`
		HardwareFeatures []byte `json:"hardwareFeatures"`
		*Alias
	}{
		Alias: (*Alias)(a),
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	if len(aux.DeviceConfig) > 0 {
		var dc device.Config
		if err := proto.Unmarshal(aux.DeviceConfig, &dc); err != nil {
			return err
		}
		a.DeviceConfig = &dc
	}
	if len(aux.HardwareFeatures) > 0 {
		var hw api.HardwareFeatures
		if err := proto.Unmarshal(aux.HardwareFeatures, &hw); err != nil {
			return err
		}
		a.HardwareFeatures = &hw
	}
	return nil
}

// ListTestsArgs is nested within Args and contains arguments used by ListTestsMode.
type ListTestsArgs struct {
	// Patterns contains patterns (either empty to list all tests, exactly one attribute expression,
	// or one or more globs) describing which tests to list.
	Patterns []string `json:"patterns,omitempty"`
}

// bundleType describes the type of tests contained in a test bundle (i.e. local or remote).
type bundleType int

const (
	localBundle bundleType = iota
	remoteBundle
)

// readArgs parses runtime arguments.
// clArgs contains command-line arguments and is typically os.Args[1:].
// args contains default values for arguments and is further updated by decoding a JSON-marshaled Args struct from stdin.
// The caller is responsible for performing the requested action.
func readArgs(clArgs []string, stdin io.Reader, stderr io.Writer, args *Args, bt bundleType) error {
	if len(clArgs) != 0 {
		flags := flag.NewFlagSet("", flag.ContinueOnError)
		flags.SetOutput(stderr)
		flags.Usage = func() {
			runner := "local_test_runner"
			if bt == remoteBundle {
				runner = "remote_test_runner"
			}
			fmt.Fprintf(stderr, "Usage: %s [flag]...\n\n"+
				"Tast test bundle containing integration tests.\n\n"+
				"This is typically executed by %s.\n\n",
				filepath.Base(os.Args[0]), runner)
			flags.PrintDefaults()
		}

		dump := flags.Bool("dumptests", false, "dump all tests as a JSON-marshaled array of testing.Test structs")
		exportMetadata := flags.Bool("exportmetadata", false, "export all test metadata as a protobuf-marshaled message")
		rpc := flags.Bool("rpc", false, "run gRPC server")
		if err := flags.Parse(clArgs); err != nil {
			return command.NewStatusErrorf(statusBadArgs, "%v", err)
		}
		if *dump {
			args.Mode = ListTestsMode
			args.ListTests = &ListTestsArgs{}
			return nil
		}
		if *exportMetadata {
			args.Mode = ExportMetadataMode
			return nil
		}
		if *rpc {
			args.Mode = RPCMode
			return nil
		}
	}

	if err := json.NewDecoder(stdin).Decode(args); err != nil {
		return command.NewStatusErrorf(statusBadArgs, "failed to decode args from stdin: %v", err)
	}

	if (args.Mode == RunTestsMode && args.RunTests == nil) ||
		(args.Mode == ListTestsMode && args.ListTests == nil) {
		return command.NewStatusErrorf(statusBadArgs, "args not set for mode %v", args.Mode)
	}

	// Use non-zero-valued deprecated fields if they were supplied by an old test runner.
	args.PromoteDeprecated()

	return nil
}
