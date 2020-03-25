// Code generated by protoc-gen-go. DO NOT EDIT.
// source: api/mfg_config.proto

package api

import (
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

// Contains 2 types of information related to an actual
// manufacturing production run of hardware.
//
// 1) Second sourced components used that do not affect system features.
//    (If it affected system features then it belongs in HardwareTopology)
// 2) Data proscribed by Google for the target run that cannot change
//    for a given piece of hardware. E.g. region code
type MfgConfig struct {
	// Unique id scoped to a Design within a Platform.
	Id *MfgConfigId `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	// Name of 2nd sourced PCB Vendor.
	PcbVendor string `protobuf:"bytes,2,opt,name=pcb_vendor,json=pcbVendor,proto3" json:"pcb_vendor,omitempty"`
	// Ram part number. The characteristics are encoded in HardwareTopology.
	RamPartNumber        string   `protobuf:"bytes,3,opt,name=ram_part_number,json=ramPartNumber,proto3" json:"ram_part_number,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *MfgConfig) Reset()         { *m = MfgConfig{} }
func (m *MfgConfig) String() string { return proto.CompactTextString(m) }
func (*MfgConfig) ProtoMessage()    {}
func (*MfgConfig) Descriptor() ([]byte, []int) {
	return fileDescriptor_e7e662181b5eea21, []int{0}
}

func (m *MfgConfig) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_MfgConfig.Unmarshal(m, b)
}
func (m *MfgConfig) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_MfgConfig.Marshal(b, m, deterministic)
}
func (m *MfgConfig) XXX_Merge(src proto.Message) {
	xxx_messageInfo_MfgConfig.Merge(m, src)
}
func (m *MfgConfig) XXX_Size() int {
	return xxx_messageInfo_MfgConfig.Size(m)
}
func (m *MfgConfig) XXX_DiscardUnknown() {
	xxx_messageInfo_MfgConfig.DiscardUnknown(m)
}

var xxx_messageInfo_MfgConfig proto.InternalMessageInfo

func (m *MfgConfig) GetId() *MfgConfigId {
	if m != nil {
		return m.Id
	}
	return nil
}

func (m *MfgConfig) GetPcbVendor() string {
	if m != nil {
		return m.PcbVendor
	}
	return ""
}

func (m *MfgConfig) GetRamPartNumber() string {
	if m != nil {
		return m.RamPartNumber
	}
	return ""
}

func init() {
	proto.RegisterType((*MfgConfig)(nil), "chromiumos.config.api.MfgConfig")
}

func init() { proto.RegisterFile("api/mfg_config.proto", fileDescriptor_e7e662181b5eea21) }

var fileDescriptor_e7e662181b5eea21 = []byte{
	// 203 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0x12, 0x49, 0x2c, 0xc8, 0xd4,
	0xcf, 0x4d, 0x4b, 0x8f, 0x4f, 0xce, 0xcf, 0x4b, 0xcb, 0x4c, 0xd7, 0x2b, 0x28, 0xca, 0x2f, 0xc9,
	0x17, 0x12, 0x4d, 0xce, 0x28, 0xca, 0xcf, 0xcd, 0x2c, 0xcd, 0xcd, 0x2f, 0xd6, 0x83, 0x4a, 0x24,
	0x16, 0x64, 0x4a, 0x89, 0xa3, 0x2a, 0x8e, 0xcf, 0x4c, 0x81, 0xa8, 0x57, 0x6a, 0x63, 0xe4, 0xe2,
	0xf4, 0x4d, 0x4b, 0x77, 0x06, 0x0b, 0x0b, 0x19, 0x71, 0x31, 0x65, 0xa6, 0x48, 0x30, 0x2a, 0x30,
	0x6a, 0x70, 0x1b, 0x29, 0xe9, 0x61, 0x35, 0x4a, 0x0f, 0xae, 0xda, 0x33, 0x25, 0x88, 0x29, 0x33,
	0x45, 0x48, 0x96, 0x8b, 0xab, 0x20, 0x39, 0x29, 0xbe, 0x2c, 0x35, 0x2f, 0x25, 0xbf, 0x48, 0x82,
	0x49, 0x81, 0x51, 0x83, 0x33, 0x88, 0xb3, 0x20, 0x39, 0x29, 0x0c, 0x2c, 0x20, 0xa4, 0xc6, 0xc5,
	0x5f, 0x94, 0x98, 0x1b, 0x5f, 0x90, 0x58, 0x54, 0x12, 0x9f, 0x57, 0x9a, 0x9b, 0x94, 0x5a, 0x24,
	0xc1, 0x0c, 0x56, 0xc3, 0x5b, 0x94, 0x98, 0x1b, 0x90, 0x58, 0x54, 0xe2, 0x07, 0x16, 0x74, 0xd2,
	0x8a, 0xd2, 0x48, 0xcf, 0x87, 0x5b, 0xa9, 0x97, 0x5f, 0x94, 0xae, 0x8f, 0xb0, 0x5f, 0x1f, 0x62,
	0xbf, 0x7e, 0x7a, 0xbe, 0x7e, 0x62, 0x41, 0x66, 0x12, 0x1b, 0xd8, 0xed, 0xc6, 0x80, 0x00, 0x00,
	0x00, 0xff, 0xff, 0xb2, 0xce, 0xac, 0x9b, 0x03, 0x01, 0x00, 0x00,
}
