package runner

import (
	"chromiumos/tast/rpc"
	"chromiumos/tast/testing"
	"context"
	"io"
	"os"
	"os/exec"

	"google.golang.org/grpc"
)

// Server implements TastCoreServiceServer for local and remote test runner.
type Server struct {
	// args given from main.go.
	// Contains RunTests.{BundleGlob, BundleArgs}
	args *Args
	// cfg given from main.go.
	// Contains SoftwareFeatureDefinitions etc.
	cfg *Config // cfg given from main.go

	// conn is the connection to the bundle's gRPC server.
	conn map[string]grpc.ClientConn
}

// List returns tests to run with skip information too.
// Unlike previous ListTestsMode, it also checks software deps and returns exactly what
// tests would be run on the bundles.
func (s *Server) List(ctx context.Context, req *rpc.ListRequest) (*rpc.ListResponse, error) {
	testing.ContextLog(ctx, "Hello ContextLog!")

	bundles, err := getBundles(req.BundleGlob)
	if err != nil {
		return nil, err
	}
	testing.ContextLog(ctx, "Got bundles: ", bundles)

	response := &rpc.ListResponse{}

	// TODO: parallelize
	for _, bundle := range bundles {
		testing.ContextLog(ctx, "Processing bundle ", bundle)

		inr, inw := io.Pipe()
		outr, outw := io.Pipe()

		// TODO: consider setting sid.
		cmd := exec.CommandContext(ctx, bundle, "-rpcv2")

		cmd.Stdout = outw
		cmd.Stderr = os.Stderr
		cmd.Stdin = inr

		if err := cmd.Start(); err != nil {
			return nil, err
		}
		go func() {
			if err := cmd.Wait(); err != nil {
				panic("Command failed: " + err.Error())
			}
		}()

		conn, err := rpc.NewPipeClientConn(ctx, outr, inw)
		if err != nil {
			return nil, err
		}

		cl := rpc.NewTastCoreServiceClient(conn)
		res, err := cl.List(ctx, req)
		if err != nil {
			return nil, err
		}

		// TODO: add information
		response.Test = append(response.Test, res.Test...)
	}
	return response, nil
}

func (*Server) Run(req *rpc.RunRequest, srv rpc.TastCoreService_RunServer) error {
	return nil
}
