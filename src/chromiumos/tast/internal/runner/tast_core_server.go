package runner

import (
	"chromiumos/tast/rpc"
	"chromiumos/tast/testing"
	"context"
	"io"
	"os"
	"os/exec"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
)

type cmdConn struct {
	cmd  *exec.Cmd
	conn *grpc.ClientConn
}

// TODO: examine this is right.
func (c *cmdConn) close() (retErr error) {
	defer func() {
		if err := func() error {
			if err := c.cmd.Process.Kill(); err != nil {
				return err
			}
			if err := c.cmd.Wait(); err != nil {
				return err
			}
			return nil
		}(); err != nil && retErr == nil {
			retErr = err
		}
	}()
	if err := c.conn.Close(); err != nil {
		return err
	}
	return
}

// Server implements TastCoreServiceServer for local and remote test runner.
type Server struct {
	// args given from main.go.
	// Contains RunTests.{BundleGlob, BundleArgs}
	args *Args
	// cfg given from main.go.
	// Contains SoftwareFeatureDefinitions etc.
	cfg *Config // cfg given from main.go

	// conn is the connection to the bundle's gRPC server.
	conn map[string]cmdConn
}

var _ rpc.TastCoreServiceServer = (*Server)(nil)

// Close should be called by the caller.
func NewTastCoreServer(args *Args, cfg *Config) *Server {
	return &Server{
		args: args,
		cfg:  cfg,
		conn: make(map[string](cmdConn)),
	}
}

func (s *Server) Close(context.Context, *empty.Empty) (_ *empty.Empty, retErr error) {
	for _, c := range s.conn {
		if err := c.close(); err != nil && retErr == nil {
			retErr = err
		}
	}
	return
}

// Dial creats connection to the bundle. Returned value is not owned by the caller; no need to Close.
func (s *Server) dial(ctx context.Context, bundle string) (*grpc.ClientConn, error) {
	if _, ok := s.conn[bundle]; ok {
		return s.conn[bundle].conn, nil
	}

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

	conn, err := rpc.NewPipeClientConn(ctx, outr, inw)
	if err != nil {
		// TODO: kill cmd.
		return nil, err
	}
	s.conn[bundle] = cmdConn{cmd: cmd, conn: conn}
	return conn, nil
}

// Bundles returns the paths to the bundles matching req.
func (s *Server) Bundles(ctx context.Context, req *rpc.BundlesRequest) (*rpc.BundlesResponse, error) {
	bundles, err := getBundles(req.BundleGlob)
	if err != nil {
		return nil, err
	}
	testing.ContextLog(ctx, "Got bundles: ", bundles)
	return &rpc.BundlesResponse{Bundles: bundles}, nil
}

// List returns tests to run with skip information too.
// Unlike previous ListTestsMode, it also checks software deps and returns exactly what
// tests would be run on the bundles.
func (s *Server) List(ctx context.Context, req *rpc.ListRequest) (*rpc.ListResponse, error) {
	bundle := req.Bundle

	testing.ContextLog(ctx, "Processing bundle ", bundle)

	conn, err := s.dial(ctx, bundle)
	if err != nil {
		return nil, err
	}
	cl := rpc.NewTastCoreServiceClient(conn)
	// TODO: add software deps information.

	res, err := cl.List(ctx, req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (*Server) Run(req *rpc.RunRequest, srv rpc.TastCoreService_RunServer) error {
	return nil
}
