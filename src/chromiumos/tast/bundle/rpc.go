package bundle

import (
	"context"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"chromiumos/tast/rpc"
	"chromiumos/tast/testing"
)

func runRPCServer(_ context.Context, stdin io.Reader, stdout io.Writer) error {
	logger := rpc.NewLoggingServerImpl()

	srv := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (res interface{}, err error) {
		ctx = logger.WithContext(ctx)
		return handler(ctx, req)
	}))

	reflection.Register(srv)
	rpc.RegisterLoggingServer(srv, logger)

	for _, s := range testing.GlobalRegistry().AllServices() {
		s.Register(srv, &testing.ServiceState{})
	}

	return srv.Serve(rpc.NewPipeListener(stdin, stdout))
}
