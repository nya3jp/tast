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
	ts, err := testsToRun(s.cfg, req.Pattern)
	if err != nil {
		return nil, err
	}
	res := &rpc.ListResponse{}
	for _, t := range ts {
		res.Test = append(res.Test, &rpc.TestInfo{
			Name: t.Name,
		})
	}
	return res, nil
}

func (*Server) Run(req *rpc.RunRequest, srv rpc.TastCoreService_RunServer) error {
	return nil
}
