package crosconfig

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func TestCheckHardwareProperty(t *testing.T) {
	origRunCrosConfig := runCrosConfig
	runCrosConfig = fakeRunCrosConfig
	defer func() { runCrosConfig = origRunCrosConfig }()

	for _, tc := range []struct {
		prop        HardwareProperty
		expected    bool
		expectError bool
	}{
		{HasBaseGyroscope, false, true},
		{HasBaseMagnetometer, false, false},
		{HasBaseAccelerometer, false, false},
		{HasLidAccelerometer, true, false},
	} {
		out, err := CheckHardwareProperty(context.Background(), tc.prop)

		if err != nil && !tc.expectError {
			t.Errorf("[%v] Expected nil error, got %#v", tc.prop, err)
		} else if out != tc.expected {
			t.Errorf("[%v] Expected out to be %v, got %v", tc.prop, tc.expected, out)
		}
	}
}

func TestCrosConfigFake(t *testing.T) {
	if os.Getenv("CROSCONFIG_CHILD") == "1" {
		// child process
		crosConfig(os.Args[3:]...)
		panic("unreachable")
	}
}

func fakeRunCrosConfig(ctx context.Context, arg ...string) ([]byte, error) {
	cs := []string{"-test.run=TestCrosConfigFake", "--"}
	cs = append(cs, arg...)
	cmd := exec.CommandContext(ctx, os.Args[0], cs...)
	cmd.Env = []string{"CROSCONFIG_CHILD=1"}
	return cmd.Output()
}

// Fake cros_config
func crosConfig(args ...string) {
	prop := args[1]

	switch prop {
	case string(HasBaseGyroscope):
		os.Exit(1)
	case string(HasBaseMagnetometer):
		fmt.Fprintf(os.Stdout, "false")
	case string(HasBaseAccelerometer):
		fmt.Fprintf(os.Stdout, "")
	default:
		fmt.Fprintf(os.Stdout, "true")
	}

	os.Exit(0)
}
