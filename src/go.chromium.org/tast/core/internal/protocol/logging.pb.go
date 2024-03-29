// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.27.1
// 	protoc        v4.23.3
// source: logging.proto

package protocol

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// LogLevel corresponds to the go type
// go.chromium.org/tast/core/internal/logging.Level
type LogLevel int32

const (
	LogLevel_LOGLEVEL_UNSPECIFIED LogLevel = 0
	LogLevel_DEBUG                LogLevel = 1
	LogLevel_INFO                 LogLevel = 2
)

// Enum value maps for LogLevel.
var (
	LogLevel_name = map[int32]string{
		0: "LOGLEVEL_UNSPECIFIED",
		1: "DEBUG",
		2: "INFO",
	}
	LogLevel_value = map[string]int32{
		"LOGLEVEL_UNSPECIFIED": 0,
		"DEBUG":                1,
		"INFO":                 2,
	}
)

func (x LogLevel) Enum() *LogLevel {
	p := new(LogLevel)
	*p = x
	return p
}

func (x LogLevel) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (LogLevel) Descriptor() protoreflect.EnumDescriptor {
	return file_logging_proto_enumTypes[0].Descriptor()
}

func (LogLevel) Type() protoreflect.EnumType {
	return &file_logging_proto_enumTypes[0]
}

func (x LogLevel) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use LogLevel.Descriptor instead.
func (LogLevel) EnumDescriptor() ([]byte, []int) {
	return file_logging_proto_rawDescGZIP(), []int{0}
}

type ReadLogsRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *ReadLogsRequest) Reset() {
	*x = ReadLogsRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_logging_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ReadLogsRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReadLogsRequest) ProtoMessage() {}

func (x *ReadLogsRequest) ProtoReflect() protoreflect.Message {
	mi := &file_logging_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReadLogsRequest.ProtoReflect.Descriptor instead.
func (*ReadLogsRequest) Descriptor() ([]byte, []int) {
	return file_logging_proto_rawDescGZIP(), []int{0}
}

type ReadLogsResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// entry is an emitted log entry. It is missing for an initial
	// ReadLogsResponse to indicate success of subscription.
	Entry *LogEntry `protobuf:"bytes,1,opt,name=entry,proto3" json:"entry,omitempty"`
}

func (x *ReadLogsResponse) Reset() {
	*x = ReadLogsResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_logging_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ReadLogsResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReadLogsResponse) ProtoMessage() {}

func (x *ReadLogsResponse) ProtoReflect() protoreflect.Message {
	mi := &file_logging_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReadLogsResponse.ProtoReflect.Descriptor instead.
func (*ReadLogsResponse) Descriptor() ([]byte, []int) {
	return file_logging_proto_rawDescGZIP(), []int{1}
}

func (x *ReadLogsResponse) GetEntry() *LogEntry {
	if x != nil {
		return x.Entry
	}
	return nil
}

type LogEntry struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Msg is a logged message.
	Msg string `protobuf:"bytes,1,opt,name=msg,proto3" json:"msg,omitempty"`
	// Seq is an ID of the log entry. It is a sequentially increasing number
	// starting from 1.
	Seq uint64 `protobuf:"varint,2,opt,name=seq,proto3" json:"seq,omitempty"`
	// The level of the log message from logging.Level.
	Level LogLevel `protobuf:"varint,3,opt,name=level,proto3,enum=tast.core.LogLevel" json:"level,omitempty"`
}

func (x *LogEntry) Reset() {
	*x = LogEntry{}
	if protoimpl.UnsafeEnabled {
		mi := &file_logging_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *LogEntry) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*LogEntry) ProtoMessage() {}

func (x *LogEntry) ProtoReflect() protoreflect.Message {
	mi := &file_logging_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use LogEntry.ProtoReflect.Descriptor instead.
func (*LogEntry) Descriptor() ([]byte, []int) {
	return file_logging_proto_rawDescGZIP(), []int{2}
}

func (x *LogEntry) GetMsg() string {
	if x != nil {
		return x.Msg
	}
	return ""
}

func (x *LogEntry) GetSeq() uint64 {
	if x != nil {
		return x.Seq
	}
	return 0
}

func (x *LogEntry) GetLevel() LogLevel {
	if x != nil {
		return x.Level
	}
	return LogLevel_LOGLEVEL_UNSPECIFIED
}

var File_logging_proto protoreflect.FileDescriptor

var file_logging_proto_rawDesc = []byte{
	0x0a, 0x0d, 0x6c, 0x6f, 0x67, 0x67, 0x69, 0x6e, 0x67, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12,
	0x09, 0x74, 0x61, 0x73, 0x74, 0x2e, 0x63, 0x6f, 0x72, 0x65, 0x22, 0x11, 0x0a, 0x0f, 0x52, 0x65,
	0x61, 0x64, 0x4c, 0x6f, 0x67, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x22, 0x3d, 0x0a,
	0x10, 0x52, 0x65, 0x61, 0x64, 0x4c, 0x6f, 0x67, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x12, 0x29, 0x0a, 0x05, 0x65, 0x6e, 0x74, 0x72, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x13, 0x2e, 0x74, 0x61, 0x73, 0x74, 0x2e, 0x63, 0x6f, 0x72, 0x65, 0x2e, 0x4c, 0x6f, 0x67,
	0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x05, 0x65, 0x6e, 0x74, 0x72, 0x79, 0x22, 0x59, 0x0a, 0x08,
	0x4c, 0x6f, 0x67, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6d, 0x73, 0x67, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6d, 0x73, 0x67, 0x12, 0x10, 0x0a, 0x03, 0x73, 0x65,
	0x71, 0x18, 0x02, 0x20, 0x01, 0x28, 0x04, 0x52, 0x03, 0x73, 0x65, 0x71, 0x12, 0x29, 0x0a, 0x05,
	0x6c, 0x65, 0x76, 0x65, 0x6c, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x13, 0x2e, 0x74, 0x61,
	0x73, 0x74, 0x2e, 0x63, 0x6f, 0x72, 0x65, 0x2e, 0x4c, 0x6f, 0x67, 0x4c, 0x65, 0x76, 0x65, 0x6c,
	0x52, 0x05, 0x6c, 0x65, 0x76, 0x65, 0x6c, 0x2a, 0x39, 0x0a, 0x08, 0x4c, 0x6f, 0x67, 0x4c, 0x65,
	0x76, 0x65, 0x6c, 0x12, 0x18, 0x0a, 0x14, 0x4c, 0x4f, 0x47, 0x4c, 0x45, 0x56, 0x45, 0x4c, 0x5f,
	0x55, 0x4e, 0x53, 0x50, 0x45, 0x43, 0x49, 0x46, 0x49, 0x45, 0x44, 0x10, 0x00, 0x12, 0x09, 0x0a,
	0x05, 0x44, 0x45, 0x42, 0x55, 0x47, 0x10, 0x01, 0x12, 0x08, 0x0a, 0x04, 0x49, 0x4e, 0x46, 0x4f,
	0x10, 0x02, 0x32, 0x54, 0x0a, 0x07, 0x4c, 0x6f, 0x67, 0x67, 0x69, 0x6e, 0x67, 0x12, 0x49, 0x0a,
	0x08, 0x52, 0x65, 0x61, 0x64, 0x4c, 0x6f, 0x67, 0x73, 0x12, 0x1a, 0x2e, 0x74, 0x61, 0x73, 0x74,
	0x2e, 0x63, 0x6f, 0x72, 0x65, 0x2e, 0x52, 0x65, 0x61, 0x64, 0x4c, 0x6f, 0x67, 0x73, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1b, 0x2e, 0x74, 0x61, 0x73, 0x74, 0x2e, 0x63, 0x6f, 0x72,
	0x65, 0x2e, 0x52, 0x65, 0x61, 0x64, 0x4c, 0x6f, 0x67, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x22, 0x00, 0x28, 0x01, 0x30, 0x01, 0x42, 0x2d, 0x5a, 0x2b, 0x67, 0x6f, 0x2e, 0x63,
	0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x74, 0x61, 0x73, 0x74,
	0x2f, 0x63, 0x6f, 0x72, 0x65, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_logging_proto_rawDescOnce sync.Once
	file_logging_proto_rawDescData = file_logging_proto_rawDesc
)

func file_logging_proto_rawDescGZIP() []byte {
	file_logging_proto_rawDescOnce.Do(func() {
		file_logging_proto_rawDescData = protoimpl.X.CompressGZIP(file_logging_proto_rawDescData)
	})
	return file_logging_proto_rawDescData
}

var file_logging_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_logging_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_logging_proto_goTypes = []interface{}{
	(LogLevel)(0),            // 0: tast.core.LogLevel
	(*ReadLogsRequest)(nil),  // 1: tast.core.ReadLogsRequest
	(*ReadLogsResponse)(nil), // 2: tast.core.ReadLogsResponse
	(*LogEntry)(nil),         // 3: tast.core.LogEntry
}
var file_logging_proto_depIdxs = []int32{
	3, // 0: tast.core.ReadLogsResponse.entry:type_name -> tast.core.LogEntry
	0, // 1: tast.core.LogEntry.level:type_name -> tast.core.LogLevel
	1, // 2: tast.core.Logging.ReadLogs:input_type -> tast.core.ReadLogsRequest
	2, // 3: tast.core.Logging.ReadLogs:output_type -> tast.core.ReadLogsResponse
	3, // [3:4] is the sub-list for method output_type
	2, // [2:3] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_logging_proto_init() }
func file_logging_proto_init() {
	if File_logging_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_logging_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ReadLogsRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_logging_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ReadLogsResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_logging_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*LogEntry); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_logging_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_logging_proto_goTypes,
		DependencyIndexes: file_logging_proto_depIdxs,
		EnumInfos:         file_logging_proto_enumTypes,
		MessageInfos:      file_logging_proto_msgTypes,
	}.Build()
	File_logging_proto = out.File
	file_logging_proto_rawDesc = nil
	file_logging_proto_goTypes = nil
	file_logging_proto_depIdxs = nil
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConnInterface

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion6

// LoggingClient is the client API for Logging service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type LoggingClient interface {
	// ReadLogs subscribes to logs emitted by gRPC services.
	// At the beginning of the call, one ReadLogsResponse with empty entry is
	// sent to indicate success of subscription. Afterwards ReadLogsResponse is
	// sent back as a stream as logs are emitted. The response stream is closed
	// when the client closes the request stream or any error occurs.
	// At most one client can have an active call of this method at a time.
	ReadLogs(ctx context.Context, opts ...grpc.CallOption) (Logging_ReadLogsClient, error)
}

type loggingClient struct {
	cc grpc.ClientConnInterface
}

func NewLoggingClient(cc grpc.ClientConnInterface) LoggingClient {
	return &loggingClient{cc}
}

func (c *loggingClient) ReadLogs(ctx context.Context, opts ...grpc.CallOption) (Logging_ReadLogsClient, error) {
	stream, err := c.cc.NewStream(ctx, &_Logging_serviceDesc.Streams[0], "/tast.core.Logging/ReadLogs", opts...)
	if err != nil {
		return nil, err
	}
	x := &loggingReadLogsClient{stream}
	return x, nil
}

type Logging_ReadLogsClient interface {
	Send(*ReadLogsRequest) error
	Recv() (*ReadLogsResponse, error)
	grpc.ClientStream
}

type loggingReadLogsClient struct {
	grpc.ClientStream
}

func (x *loggingReadLogsClient) Send(m *ReadLogsRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *loggingReadLogsClient) Recv() (*ReadLogsResponse, error) {
	m := new(ReadLogsResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// LoggingServer is the server API for Logging service.
type LoggingServer interface {
	// ReadLogs subscribes to logs emitted by gRPC services.
	// At the beginning of the call, one ReadLogsResponse with empty entry is
	// sent to indicate success of subscription. Afterwards ReadLogsResponse is
	// sent back as a stream as logs are emitted. The response stream is closed
	// when the client closes the request stream or any error occurs.
	// At most one client can have an active call of this method at a time.
	ReadLogs(Logging_ReadLogsServer) error
}

// UnimplementedLoggingServer can be embedded to have forward compatible implementations.
type UnimplementedLoggingServer struct {
}

func (*UnimplementedLoggingServer) ReadLogs(Logging_ReadLogsServer) error {
	return status.Errorf(codes.Unimplemented, "method ReadLogs not implemented")
}

func RegisterLoggingServer(s *grpc.Server, srv LoggingServer) {
	s.RegisterService(&_Logging_serviceDesc, srv)
}

func _Logging_ReadLogs_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(LoggingServer).ReadLogs(&loggingReadLogsServer{stream})
}

type Logging_ReadLogsServer interface {
	Send(*ReadLogsResponse) error
	Recv() (*ReadLogsRequest, error)
	grpc.ServerStream
}

type loggingReadLogsServer struct {
	grpc.ServerStream
}

func (x *loggingReadLogsServer) Send(m *ReadLogsResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *loggingReadLogsServer) Recv() (*ReadLogsRequest, error) {
	m := new(ReadLogsRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

var _Logging_serviceDesc = grpc.ServiceDesc{
	ServiceName: "tast.core.Logging",
	HandlerType: (*LoggingServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "ReadLogs",
			Handler:       _Logging_ReadLogs_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "logging.proto",
}
