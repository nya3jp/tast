package crosconfig

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
)

// Mock cros_config
func main() {
	prop := os.Args[3]

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

func TestCheckHardwareProperty(t *testing.T) {

	if os.Getenv("CROSCONFIG_CHILD") == "1" {
		// child process
		os.Args = os.Args[2:]
		main()
		panic("unreachable")
	}

	checkValue := func(prop HardwareProperty, expected bool, expectError bool) {
		execCommandContext = func(ctx context.Context, command string, args ...string) *exec.Cmd {
			cs := []string{"-test.run=TestCheckHardwareProperty", "--", command}
			cs = append(cs, args...)
			cmd := exec.CommandContext(ctx, os.Args[0], cs...)
			cmd.Env = []string{"CROSCONFIG_CHILD=1"}
			return cmd
		}
		defer func() { execCommandContext = exec.CommandContext }()
		out, err := CheckHardwareProperty(context.Background(), prop)

		if err != nil && !expectError {
			t.Errorf("[%v] Expected nil error, got %#v", prop, err)
		} else if out != expected {
			t.Errorf("[%v] Expected out to be %v, got %v", prop, expected, out)
		}
	}

	checkValue(HasBaseGyroscope, false, true)
	checkValue(HasBaseMagnetometer, false, false)
	checkValue(HasBaseAccelerometer, false, false)
	checkValue(HasLidAccelerometer, true, false)
}
