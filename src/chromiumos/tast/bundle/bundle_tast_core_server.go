package bundle

import (
	"chromiumos/tast/rpc"
	"context"
)

// Server implements TastCoreServiceServer for local and remote bundle.
type Server struct {
	// args passed from the runner program.
	args *Args
	cfg  *runConfig
}

var _ rpc.TastCoreServiceServer = (*Server)(nil)

// List returns tests to run with skip information too.
// Unlike previous ListTestsMode, it also checks software deps and returns exactly what
// tests would be run on the bundles.
func (s *Server) List(ctx context.Context, req *rpc.ListRequest) (*rpc.ListResponse, error) {
	testsToRun(s.cfg, req.Pattern)
	return nil, nil
}

func (*Server) Run(req *rpc.RunRequest, srv rpc.TastCoreService_RunServer) error {
	return nil
}
