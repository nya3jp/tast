package bundle

import (
	"chromiumos/tast/rpc"
	"chromiumos/tast/testing"
	"context"

	"github.com/golang/protobuf/ptypes/empty"
)

// Server implements TastCoreServiceServer for local and remote bundle.
type Server struct {
	// args passed from the runner program.
	args *Args
	cfg  *runConfig
}

var _ rpc.TastCoreServiceServer = (*Server)(nil)

func (s *Server) Close(context.Context, *empty.Empty) (*empty.Empty, error) {
	return nil, nil
}

func (s *Server) Bundles(context.Context, *rpc.BundlesRequest) (*rpc.BundlesResponse, error) {
	panic("not implemented")
}

// List returns tests to run with skip information too.
// Unlike previous ListTestsMode, it also checks software deps and returns exactly what
// tests would be run on the bundles.
// It also returns all the preconditions defined in the bundle.
func (s *Server) List(ctx context.Context, req *rpc.ListRequest) (*rpc.ListResponse, error) {
	ts, err := testsToRun(s.cfg, req.Pattern)
	if err != nil {
		return nil, err
	}
	res := &rpc.ListResponse{}
	for _, t := range ts {
		res.Tests = append(res.Tests, &rpc.RawTestInfo{
			Name:         t.Name,
			Precondition: t.PreV2,
		})
	}
	pres := testing.GlobalRegistry().AllPreconditionV2s()
	for name, p := range pres {
		res.Preconditions = append(res.Preconditions, &rpc.RawPrecondition{
			Name:   name,
			Parent: p.Parent(),
		})
	}
	return res, nil
}

func (*Server) Run(req *rpc.RunRequest, srv rpc.TastCoreService_RunServer) error {
	return nil
}
