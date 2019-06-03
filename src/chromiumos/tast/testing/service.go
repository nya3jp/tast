package testing

import (
	"google.golang.org/grpc"
)

type Service struct {
	Data []string `json:"data,omitempty"`

	Register func(srv *grpc.Server, s *ServiceState) `json:"-"`
}

type ServiceState struct{}
