// Copyright 2021 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.27.1
// 	protoc        v3.11.4
// source: reports.proto

package protocol

import (
	context "context"
	duration "github.com/golang/protobuf/ptypes/duration"
	empty "github.com/golang/protobuf/ptypes/empty"
	timestamp "github.com/golang/protobuf/ptypes/timestamp"
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

type LogStreamRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Test    string `protobuf:"bytes,1,opt,name=test,proto3" json:"test,omitempty"`                      // test name of this log message
	LogPath string `protobuf:"bytes,2,opt,name=log_path,json=logPath,proto3" json:"log_path,omitempty"` // test log file path relative to the result directory
	Data    []byte `protobuf:"bytes,3,opt,name=data,proto3" json:"data,omitempty"`
}

func (x *LogStreamRequest) Reset() {
	*x = LogStreamRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_reports_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *LogStreamRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*LogStreamRequest) ProtoMessage() {}

func (x *LogStreamRequest) ProtoReflect() protoreflect.Message {
	mi := &file_reports_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use LogStreamRequest.ProtoReflect.Descriptor instead.
func (*LogStreamRequest) Descriptor() ([]byte, []int) {
	return file_reports_proto_rawDescGZIP(), []int{0}
}

func (x *LogStreamRequest) GetTest() string {
	if x != nil {
		return x.Test
	}
	return ""
}

func (x *LogStreamRequest) GetLogPath() string {
	if x != nil {
		return x.LogPath
	}
	return ""
}

func (x *LogStreamRequest) GetData() []byte {
	if x != nil {
		return x.Data
	}
	return nil
}

type ReportResultRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Test string `protobuf:"bytes,1,opt,name=test,proto3" json:"test,omitempty"` // test name of this test result
	// errors contain errors encountered while running the test.
	Errors []*ErrorReport `protobuf:"bytes,2,rep,name=errors,proto3" json:"errors,omitempty"`
	// skip_reason tells why the test is skipped.
	SkipReason string `protobuf:"bytes,3,opt,name=skip_reason,json=skipReason,proto3" json:"skip_reason,omitempty"`
	// start_time tells the start running time of the test.
	StartTime *timestamp.Timestamp `protobuf:"bytes,4,opt,name=start_time,json=startTime,proto3" json:"start_time,omitempty"`
	// duration tells the duration of the test run.
	Duration *duration.Duration `protobuf:"bytes,5,opt,name=duration,proto3" json:"duration,omitempty"`
}

func (x *ReportResultRequest) Reset() {
	*x = ReportResultRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_reports_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ReportResultRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReportResultRequest) ProtoMessage() {}

func (x *ReportResultRequest) ProtoReflect() protoreflect.Message {
	mi := &file_reports_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReportResultRequest.ProtoReflect.Descriptor instead.
func (*ReportResultRequest) Descriptor() ([]byte, []int) {
	return file_reports_proto_rawDescGZIP(), []int{1}
}

func (x *ReportResultRequest) GetTest() string {
	if x != nil {
		return x.Test
	}
	return ""
}

func (x *ReportResultRequest) GetErrors() []*ErrorReport {
	if x != nil {
		return x.Errors
	}
	return nil
}

func (x *ReportResultRequest) GetSkipReason() string {
	if x != nil {
		return x.SkipReason
	}
	return ""
}

func (x *ReportResultRequest) GetStartTime() *timestamp.Timestamp {
	if x != nil {
		return x.StartTime
	}
	return nil
}

func (x *ReportResultRequest) GetDuration() *duration.Duration {
	if x != nil {
		return x.Duration
	}
	return nil
}

type ReportResultResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Terminate bool `protobuf:"varint,1,opt,name=terminate,proto3" json:"terminate,omitempty"` // If set, the tast should, skipping remaining tests.
}

func (x *ReportResultResponse) Reset() {
	*x = ReportResultResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_reports_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ReportResultResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReportResultResponse) ProtoMessage() {}

func (x *ReportResultResponse) ProtoReflect() protoreflect.Message {
	mi := &file_reports_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReportResultResponse.ProtoReflect.Descriptor instead.
func (*ReportResultResponse) Descriptor() ([]byte, []int) {
	return file_reports_proto_rawDescGZIP(), []int{2}
}

func (x *ReportResultResponse) GetTerminate() bool {
	if x != nil {
		return x.Terminate
	}
	return false
}

type ErrorReport struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Time   *timestamp.Timestamp `protobuf:"bytes,1,opt,name=time,proto3" json:"time,omitempty"`
	Reason string               `protobuf:"bytes,2,opt,name=reason,proto3" json:"reason,omitempty"`
	File   string               `protobuf:"bytes,3,opt,name=file,proto3" json:"file,omitempty"`
	Line   int32                `protobuf:"varint,4,opt,name=line,proto3" json:"line,omitempty"`
	Stack  string               `protobuf:"bytes,5,opt,name=stack,proto3" json:"stack,omitempty"`
}

func (x *ErrorReport) Reset() {
	*x = ErrorReport{}
	if protoimpl.UnsafeEnabled {
		mi := &file_reports_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ErrorReport) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ErrorReport) ProtoMessage() {}

func (x *ErrorReport) ProtoReflect() protoreflect.Message {
	mi := &file_reports_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ErrorReport.ProtoReflect.Descriptor instead.
func (*ErrorReport) Descriptor() ([]byte, []int) {
	return file_reports_proto_rawDescGZIP(), []int{3}
}

func (x *ErrorReport) GetTime() *timestamp.Timestamp {
	if x != nil {
		return x.Time
	}
	return nil
}

func (x *ErrorReport) GetReason() string {
	if x != nil {
		return x.Reason
	}
	return ""
}

func (x *ErrorReport) GetFile() string {
	if x != nil {
		return x.File
	}
	return ""
}

func (x *ErrorReport) GetLine() int32 {
	if x != nil {
		return x.Line
	}
	return 0
}

func (x *ErrorReport) GetStack() string {
	if x != nil {
		return x.Stack
	}
	return ""
}

var File_reports_proto protoreflect.FileDescriptor

var file_reports_proto_rawDesc = []byte{
	0x0a, 0x0d, 0x72, 0x65, 0x70, 0x6f, 0x72, 0x74, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12,
	0x09, 0x74, 0x61, 0x73, 0x74, 0x2e, 0x63, 0x6f, 0x72, 0x65, 0x1a, 0x1e, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x64, 0x75, 0x72, 0x61,
	0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1b, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x65, 0x6d, 0x70, 0x74,
	0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61,
	0x6d, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x55, 0x0a, 0x10, 0x4c, 0x6f, 0x67, 0x53,
	0x74, 0x72, 0x65, 0x61, 0x6d, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x12, 0x0a, 0x04,
	0x74, 0x65, 0x73, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x74, 0x65, 0x73, 0x74,
	0x12, 0x19, 0x0a, 0x08, 0x6c, 0x6f, 0x67, 0x5f, 0x70, 0x61, 0x74, 0x68, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x07, 0x6c, 0x6f, 0x67, 0x50, 0x61, 0x74, 0x68, 0x12, 0x12, 0x0a, 0x04, 0x64,
	0x61, 0x74, 0x61, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x04, 0x64, 0x61, 0x74, 0x61, 0x22,
	0xec, 0x01, 0x0a, 0x13, 0x52, 0x65, 0x70, 0x6f, 0x72, 0x74, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x74, 0x65, 0x73, 0x74, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x74, 0x65, 0x73, 0x74, 0x12, 0x2e, 0x0a, 0x06, 0x65,
	0x72, 0x72, 0x6f, 0x72, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x16, 0x2e, 0x74, 0x61,
	0x73, 0x74, 0x2e, 0x63, 0x6f, 0x72, 0x65, 0x2e, 0x45, 0x72, 0x72, 0x6f, 0x72, 0x52, 0x65, 0x70,
	0x6f, 0x72, 0x74, 0x52, 0x06, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x73, 0x12, 0x1f, 0x0a, 0x0b, 0x73,
	0x6b, 0x69, 0x70, 0x5f, 0x72, 0x65, 0x61, 0x73, 0x6f, 0x6e, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x0a, 0x73, 0x6b, 0x69, 0x70, 0x52, 0x65, 0x61, 0x73, 0x6f, 0x6e, 0x12, 0x39, 0x0a, 0x0a,
	0x73, 0x74, 0x61, 0x72, 0x74, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62,
	0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x52, 0x09, 0x73, 0x74,
	0x61, 0x72, 0x74, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x35, 0x0a, 0x08, 0x64, 0x75, 0x72, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72, 0x61,
	0x74, 0x69, 0x6f, 0x6e, 0x52, 0x08, 0x64, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x22, 0x34,
	0x0a, 0x14, 0x52, 0x65, 0x70, 0x6f, 0x72, 0x74, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x1c, 0x0a, 0x09, 0x74, 0x65, 0x72, 0x6d, 0x69, 0x6e,
	0x61, 0x74, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x08, 0x52, 0x09, 0x74, 0x65, 0x72, 0x6d, 0x69,
	0x6e, 0x61, 0x74, 0x65, 0x22, 0x93, 0x01, 0x0a, 0x0b, 0x45, 0x72, 0x72, 0x6f, 0x72, 0x52, 0x65,
	0x70, 0x6f, 0x72, 0x74, 0x12, 0x2e, 0x0a, 0x04, 0x74, 0x69, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x52, 0x04,
	0x74, 0x69, 0x6d, 0x65, 0x12, 0x16, 0x0a, 0x06, 0x72, 0x65, 0x61, 0x73, 0x6f, 0x6e, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x72, 0x65, 0x61, 0x73, 0x6f, 0x6e, 0x12, 0x12, 0x0a, 0x04,
	0x66, 0x69, 0x6c, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x66, 0x69, 0x6c, 0x65,
	0x12, 0x12, 0x0a, 0x04, 0x6c, 0x69, 0x6e, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x05, 0x52, 0x04,
	0x6c, 0x69, 0x6e, 0x65, 0x12, 0x14, 0x0a, 0x05, 0x73, 0x74, 0x61, 0x63, 0x6b, 0x18, 0x05, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x05, 0x73, 0x74, 0x61, 0x63, 0x6b, 0x32, 0xa2, 0x01, 0x0a, 0x07, 0x52,
	0x65, 0x70, 0x6f, 0x72, 0x74, 0x73, 0x12, 0x44, 0x0a, 0x09, 0x4c, 0x6f, 0x67, 0x53, 0x74, 0x72,
	0x65, 0x61, 0x6d, 0x12, 0x1b, 0x2e, 0x74, 0x61, 0x73, 0x74, 0x2e, 0x63, 0x6f, 0x72, 0x65, 0x2e,
	0x4c, 0x6f, 0x67, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62,
	0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x22, 0x00, 0x28, 0x01, 0x12, 0x51, 0x0a, 0x0c,
	0x52, 0x65, 0x70, 0x6f, 0x72, 0x74, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x12, 0x1e, 0x2e, 0x74,
	0x61, 0x73, 0x74, 0x2e, 0x63, 0x6f, 0x72, 0x65, 0x2e, 0x52, 0x65, 0x70, 0x6f, 0x72, 0x74, 0x52,
	0x65, 0x73, 0x75, 0x6c, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1f, 0x2e, 0x74,
	0x61, 0x73, 0x74, 0x2e, 0x63, 0x6f, 0x72, 0x65, 0x2e, 0x52, 0x65, 0x70, 0x6f, 0x72, 0x74, 0x52,
	0x65, 0x73, 0x75, 0x6c, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x42,
	0x24, 0x5a, 0x22, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x6f, 0x73, 0x2f, 0x74, 0x61,
	0x73, 0x74, 0x2f, 0x66, 0x72, 0x61, 0x6d, 0x65, 0x77, 0x6f, 0x72, 0x6b, 0x2f, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x63, 0x6f, 0x6c, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_reports_proto_rawDescOnce sync.Once
	file_reports_proto_rawDescData = file_reports_proto_rawDesc
)

func file_reports_proto_rawDescGZIP() []byte {
	file_reports_proto_rawDescOnce.Do(func() {
		file_reports_proto_rawDescData = protoimpl.X.CompressGZIP(file_reports_proto_rawDescData)
	})
	return file_reports_proto_rawDescData
}

var file_reports_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_reports_proto_goTypes = []interface{}{
	(*LogStreamRequest)(nil),     // 0: tast.core.LogStreamRequest
	(*ReportResultRequest)(nil),  // 1: tast.core.ReportResultRequest
	(*ReportResultResponse)(nil), // 2: tast.core.ReportResultResponse
	(*ErrorReport)(nil),          // 3: tast.core.ErrorReport
	(*timestamp.Timestamp)(nil),  // 4: google.protobuf.Timestamp
	(*duration.Duration)(nil),    // 5: google.protobuf.Duration
	(*empty.Empty)(nil),          // 6: google.protobuf.Empty
}
var file_reports_proto_depIdxs = []int32{
	3, // 0: tast.core.ReportResultRequest.errors:type_name -> tast.core.ErrorReport
	4, // 1: tast.core.ReportResultRequest.start_time:type_name -> google.protobuf.Timestamp
	5, // 2: tast.core.ReportResultRequest.duration:type_name -> google.protobuf.Duration
	4, // 3: tast.core.ErrorReport.time:type_name -> google.protobuf.Timestamp
	0, // 4: tast.core.Reports.LogStream:input_type -> tast.core.LogStreamRequest
	1, // 5: tast.core.Reports.ReportResult:input_type -> tast.core.ReportResultRequest
	6, // 6: tast.core.Reports.LogStream:output_type -> google.protobuf.Empty
	2, // 7: tast.core.Reports.ReportResult:output_type -> tast.core.ReportResultResponse
	6, // [6:8] is the sub-list for method output_type
	4, // [4:6] is the sub-list for method input_type
	4, // [4:4] is the sub-list for extension type_name
	4, // [4:4] is the sub-list for extension extendee
	0, // [0:4] is the sub-list for field type_name
}

func init() { file_reports_proto_init() }
func file_reports_proto_init() {
	if File_reports_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_reports_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*LogStreamRequest); i {
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
		file_reports_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ReportResultRequest); i {
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
		file_reports_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ReportResultResponse); i {
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
		file_reports_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ErrorReport); i {
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
			RawDescriptor: file_reports_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_reports_proto_goTypes,
		DependencyIndexes: file_reports_proto_depIdxs,
		MessageInfos:      file_reports_proto_msgTypes,
	}.Build()
	File_reports_proto = out.File
	file_reports_proto_rawDesc = nil
	file_reports_proto_goTypes = nil
	file_reports_proto_depIdxs = nil
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConnInterface

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion6

// ReportsClient is the client API for Reports service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type ReportsClient interface {
	// LogStream accepts a stream of log data.
	// The request should be called only once per client.
	LogStream(ctx context.Context, opts ...grpc.CallOption) (Reports_LogStreamClient, error)
	// ReportResults accepts test results from a client.
	// This request can be called multiple times per client.
	ReportResult(ctx context.Context, in *ReportResultRequest, opts ...grpc.CallOption) (*ReportResultResponse, error)
}

type reportsClient struct {
	cc grpc.ClientConnInterface
}

func NewReportsClient(cc grpc.ClientConnInterface) ReportsClient {
	return &reportsClient{cc}
}

func (c *reportsClient) LogStream(ctx context.Context, opts ...grpc.CallOption) (Reports_LogStreamClient, error) {
	stream, err := c.cc.NewStream(ctx, &_Reports_serviceDesc.Streams[0], "/tast.core.Reports/LogStream", opts...)
	if err != nil {
		return nil, err
	}
	x := &reportsLogStreamClient{stream}
	return x, nil
}

type Reports_LogStreamClient interface {
	Send(*LogStreamRequest) error
	CloseAndRecv() (*empty.Empty, error)
	grpc.ClientStream
}

type reportsLogStreamClient struct {
	grpc.ClientStream
}

func (x *reportsLogStreamClient) Send(m *LogStreamRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *reportsLogStreamClient) CloseAndRecv() (*empty.Empty, error) {
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	m := new(empty.Empty)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *reportsClient) ReportResult(ctx context.Context, in *ReportResultRequest, opts ...grpc.CallOption) (*ReportResultResponse, error) {
	out := new(ReportResultResponse)
	err := c.cc.Invoke(ctx, "/tast.core.Reports/ReportResult", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ReportsServer is the server API for Reports service.
type ReportsServer interface {
	// LogStream accepts a stream of log data.
	// The request should be called only once per client.
	LogStream(Reports_LogStreamServer) error
	// ReportResults accepts test results from a client.
	// This request can be called multiple times per client.
	ReportResult(context.Context, *ReportResultRequest) (*ReportResultResponse, error)
}

// UnimplementedReportsServer can be embedded to have forward compatible implementations.
type UnimplementedReportsServer struct {
}

func (*UnimplementedReportsServer) LogStream(Reports_LogStreamServer) error {
	return status.Errorf(codes.Unimplemented, "method LogStream not implemented")
}
func (*UnimplementedReportsServer) ReportResult(context.Context, *ReportResultRequest) (*ReportResultResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ReportResult not implemented")
}

func RegisterReportsServer(s *grpc.Server, srv ReportsServer) {
	s.RegisterService(&_Reports_serviceDesc, srv)
}

func _Reports_LogStream_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(ReportsServer).LogStream(&reportsLogStreamServer{stream})
}

type Reports_LogStreamServer interface {
	SendAndClose(*empty.Empty) error
	Recv() (*LogStreamRequest, error)
	grpc.ServerStream
}

type reportsLogStreamServer struct {
	grpc.ServerStream
}

func (x *reportsLogStreamServer) SendAndClose(m *empty.Empty) error {
	return x.ServerStream.SendMsg(m)
}

func (x *reportsLogStreamServer) Recv() (*LogStreamRequest, error) {
	m := new(LogStreamRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _Reports_ReportResult_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ReportResultRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ReportsServer).ReportResult(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/tast.core.Reports/ReportResult",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ReportsServer).ReportResult(ctx, req.(*ReportResultRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _Reports_serviceDesc = grpc.ServiceDesc{
	ServiceName: "tast.core.Reports",
	HandlerType: (*ReportsServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ReportResult",
			Handler:    _Reports_ReportResult_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "LogStream",
			Handler:       _Reports_LogStream_Handler,
			ClientStreams: true,
		},
	},
	Metadata: "reports.proto",
}
