# Tast Codelab: Chrome UI Automation (go/tast-codelab-3)

> This document assumes that you've already gone through [Codelab #1].

This codelab follows the creation of a remote Tast test and a grpc service
used by the test.

[Codelab #1]: codelab_1.md


## Background

Remote tests are tests which do not run on the DuT itself but on a remote
device. They are needed when a test needs to reboot during the test
execution for any reason.
The remote device can communicate with the DuT via grpc services during
the test. These services are used to execute code on the DuT itself.
In general the remote test can thus be seperated into two parts:

  1 The remote part which includes all the intialization steps and calls to
    grpc services.
  2 One ore more grpc service which executes the required code on the DuT,
    i.e. for the policy setup, login and the test logic.


## GRPC services

The grpc services are created using protocol buffers to define their methods
and their respective request and response types.

```
syntax = "proto3";

package tast.cros.policy;

import "google/protobuf/empty.proto";

option go_package = "chromiumos/tast/services/cros/policy";

// SystemTimezoneService provides a function to test the system timezone.
service SystemTimezoneService {
  rpc TestSystemTimezone(TestSystemTimezoneRequest) returns (google.protobuf.Empty) {}
}

message TestSystemTimezoneRequest {
  string Timezone = 1;
}
```
Create your service in the corresponding `folder` in
`tast-tests/src/chromiumos/tast/services/cros`. So if you need a service for a
policy test the `folder` is `policy`.
To generate the go code for your service add its proto file to the gen.go file
in the `folder`.

```
// Package policy provides the PolicyService
package policy

// Run the following command in CrOS chroot to regenerate protocol buffer bindings:
//
// ~/trunk/src/platform/tast/tools/go.sh generate chromiumos/tast/services/cros/policy
//go:generate protoc -I . --go_out=plugins=grpc:../../../../.. system_timezone.proto
```

Then run `~/trunk/src/platform/tast/tools/go.sh generate chromiumos/tast/services/cros/folder`.
This will generate a `.pb.go` file with the name of your service.

## Local service implementation
Next we need to implement what the `GetTimeInTimezone` method is actually doing
on the DuT. The service implementation is placed in the same folder as
respective local tests. So again if you are writing a remote policy test, all
created services for that test go in the `tast-tests/local_tests/policy`
folder.

For the implementation we need to import the respective package we generated
our service in (the on referred to in the `gen.go` file) as well as grpc
package. If we use empty request or response parameters in our service we also
need the `protobuf/ptypes/empty` package.

```
import (
	...
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	pb "chromiumos/tast/services/cros/policy"
	...
)
```

In the `init()` function we now add a service instead of a test:

```
func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterSystemTimezoneServiceServer(srv, &SystemTimezoneService{s: s})
		},
	})
}
```

Then we need a struct to hold our service:

```
// SystemTimezoneService implements tast.cros.policy.SystemTimezoneService.
type SystemTimezoneService struct { // NOLINT
	s *testing.ServiceState
}
```

And finally we need an implementation for all methods defined in our service.
These methods are the part of the remote test that get executed on the DuT,
so we can put most of our test logic in these:

```
func (c *SystemTimezoneService) TestSystemTimezone(ctx context.Context, req *pb.TestSystemTimezoneRequest) (*empty.Empty, error) {

	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		return nil, errors.Wrap(err, "failed to log out")
	}

	// Wait until the timezone is set.
	if err := testing.Poll(ctx, func(ctx context.Context) error {

		out, err := testexec.CommandContext(ctx, "/bin/ls", "-l", "/var/lib/timezone/localtime").Output()
		if err != nil {
			return errors.Wrap(err, "failed to get the timezone")
		}
		outStr := strings.TrimSpace(string(out))

		if !strings.Contains(outStr, req.Timezone) {
			return errors.Errorf("unexpected timezone: got %q; want %q", outStr, req.Timezone)
		}

		return nil

	}, &testing.PollOptions{
		Timeout: 30 * time.Second,
	}); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}
```

In this example the `TestSystemTimezone()` method will poll the currently set
timezone on the console and compare it to the timezone in the input parameter.
If they don't match when the timeout is reached an error is returned,
indicating that the timezone on the DuT is not the expected timezone.

## Remote test
With the generated service we can now implement the remote part of the test.
For that we import all packages containing services we need as well as the rpc
package. For tests involving enrollment we also need the `remote/policyutil`
package.

```go
import (
	...
        "chromiumos/tast/remote/policyutil"
	"chromiumos/tast/rpc"
	ps "chromiumos/tast/services/cros/policy"
	...
)
```

In the `init()` function we add the services we want to use as ServiceDeps.
If the test performs an enrollment then we also add it to the enrollment group.

```
func init() {
	testing.AddTest(&testing.Test{
		Func: SystemTimezone,
		Desc: "Just getting the time in a certain timezone",
		Contacts: []string{
			"googler@google.com", // Test author
			"chromeos-commercial-stability@google.com",
		},
		Attr:         []string{"group:enrollment"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.policy.PolicyService", "tast.cros.policy.SystemTimezoneService"},
		Timeout:      7 * time.Minute,
	})
}
```

In the test function we then establish a grpc connection to the DuT, we
create a client instance of our service on the connection and then we can call
the methods of the service.
You may also want to power wash the DuT before and after the test and enroll
it if needed.

```
func SystemTimezone(ctx context.Context, s *testing.State) {

	// Power wash the DuT after the test has finished.
	defer func(ctx context.Context) {
		if err := policyutil.EnsureTPMIsResetAndPowerwash(ctx, s.DUT()); err != nil {
			s.Error("Failed to reset TPM: ", err)
		}
	}(ctx)

	// Give the cleanup some time after the test finishes.
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()


	// Power wash the DuT before running the test.
	if err := policyutil.EnsureTPMIsResetAndPowerwash(ctx, s.DUT()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}

	// Connect to the DuT via grpc.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint(), "cros")
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	// Create client instance of the Policy service.
	pc := ps.NewPolicyServiceClient(cl.Conn)

	// Create a policy blob and enroll the device with it.
	pb := fakedms.NewPolicyBlob()
	pb.AddPolicy(&policy.SystemTimezone{Val: "Europe/Berlin"})

	pJSON, err := json.Marshal(pb)
	if err != nil {
		s.Fatal("Failed to serialize policies: ", err)
	}

	// Use the Policy service to enroll the device with the policy blob.
	if _, err := pc.EnrollUsingChrome(ctx, &ps.EnrollUsingChromeRequest{
		PolicyJson: pJSON,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}
	pc.StopChromeAndFakeDMS(ctx, &empty.Empty{})

	// Create client instance of the SystemTimezone service.
	psc := ps.NewSystemTimezoneServiceClient(cl.Conn)

	// Use the TestSystemTimezone method of the SystemTimezone service
	// to check if the timezone was set correctly by the policy.
	if _, err = psc.TestSystemTimezone(ctx, &ps.TestSystemTimezoneRequest{
		Timezone: "Europe/Berlin",
	}); err != nil {
		s.Error("Failed to set SystemTimezone policy : ", err)
	}
```
