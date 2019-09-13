// Code generated by protoc-gen-go. DO NOT EDIT.
// source: ping.proto

/*
Package rpc is a generated protocol buffer package.

It is generated from these files:
	ping.proto
	logging.proto

It has these top-level messages:
	PingRequest
	PingResponse
	ReadLogsRequest
	ReadLogsResponse
	LogEntry
*/
package rpc

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"

import (
	context "golang.org/x/net/context"
	grpc "google.golang.org/grpc"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

type PingRequest struct {
}

func (m *PingRequest) Reset()                    { *m = PingRequest{} }
func (m *PingRequest) String() string            { return proto.CompactTextString(m) }
func (*PingRequest) ProtoMessage()               {}
func (*PingRequest) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{0} }

type PingResponse struct {
}

func (m *PingResponse) Reset()                    { *m = PingResponse{} }
func (m *PingResponse) String() string            { return proto.CompactTextString(m) }
func (*PingResponse) ProtoMessage()               {}
func (*PingResponse) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{1} }

func init() {
	proto.RegisterType((*PingRequest)(nil), "tast.core.PingRequest")
	proto.RegisterType((*PingResponse)(nil), "tast.core.PingResponse")
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// Client API for Ping service

type PingClient interface {
	Ping(ctx context.Context, in *PingRequest, opts ...grpc.CallOption) (*PingResponse, error)
}

type pingClient struct {
	cc *grpc.ClientConn
}

func NewPingClient(cc *grpc.ClientConn) PingClient {
	return &pingClient{cc}
}

func (c *pingClient) Ping(ctx context.Context, in *PingRequest, opts ...grpc.CallOption) (*PingResponse, error) {
	out := new(PingResponse)
	err := grpc.Invoke(ctx, "/tast.core.Ping/Ping", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Server API for Ping service

type PingServer interface {
	Ping(context.Context, *PingRequest) (*PingResponse, error)
}

func RegisterPingServer(s *grpc.Server, srv PingServer) {
	s.RegisterService(&_Ping_serviceDesc, srv)
}

func _Ping_Ping_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(PingRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PingServer).Ping(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/tast.core.Ping/Ping",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PingServer).Ping(ctx, req.(*PingRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _Ping_serviceDesc = grpc.ServiceDesc{
	ServiceName: "tast.core.Ping",
	HandlerType: (*PingServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Ping",
			Handler:    _Ping_Ping_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "ping.proto",
}

func init() { proto.RegisterFile("ping.proto", fileDescriptor0) }

var fileDescriptor0 = []byte{
	// 125 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0xe2, 0x2a, 0xc8, 0xcc, 0x4b,
	0xd7, 0x2b, 0x28, 0xca, 0x2f, 0xc9, 0x17, 0xe2, 0x2c, 0x49, 0x2c, 0x2e, 0xd1, 0x4b, 0xce, 0x2f,
	0x4a, 0x55, 0xe2, 0xe5, 0xe2, 0x0e, 0xc8, 0xcc, 0x4b, 0x0f, 0x4a, 0x2d, 0x2c, 0x4d, 0x2d, 0x2e,
	0x51, 0xe2, 0xe3, 0xe2, 0x81, 0x70, 0x8b, 0x0b, 0xf2, 0xf3, 0x8a, 0x53, 0x8d, 0x1c, 0xb9, 0x58,
	0x40, 0x7c, 0x21, 0x4b, 0x28, 0x2d, 0xa6, 0x07, 0xd7, 0xaa, 0x87, 0xa4, 0x4f, 0x4a, 0x1c, 0x43,
	0x1c, 0x62, 0x80, 0x12, 0x83, 0x93, 0x68, 0x94, 0x70, 0x72, 0x46, 0x51, 0x7e, 0x6e, 0x66, 0x69,
	0x6e, 0x7e, 0xb1, 0x3e, 0x48, 0x99, 0x7e, 0x51, 0x41, 0x72, 0x12, 0x1b, 0xd8, 0x29, 0xc6, 0x80,
	0x00, 0x00, 0x00, 0xff, 0xff, 0x86, 0x55, 0xcf, 0xa0, 0x98, 0x00, 0x00, 0x00,
}
