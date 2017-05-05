// Code generated by protoc-gen-gogo.
// source: protos/typesp/types.proto
// DO NOT EDIT!

/*
	Package typesp is a generated protocol buffer package.

	It is generated from these files:
		protos/typesp/types.proto

	It has these top-level messages:
		Posting
		PostingList
		Schema
*/
package typesp

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import facetsp "github.com/dgraph-io/dgraph/protos/facetsp"

import io "io"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

type Posting_ValType int32

const (
	Posting_DEFAULT  Posting_ValType = 0
	Posting_BINARY   Posting_ValType = 1
	Posting_INT      Posting_ValType = 2
	Posting_FLOAT    Posting_ValType = 3
	Posting_BOOL     Posting_ValType = 4
	Posting_DATE     Posting_ValType = 5
	Posting_DATETIME Posting_ValType = 6
	Posting_GEO      Posting_ValType = 7
	Posting_UID      Posting_ValType = 8
	Posting_PASSWORD Posting_ValType = 9
	Posting_STRING   Posting_ValType = 10
)

var Posting_ValType_name = map[int32]string{
	0:  "DEFAULT",
	1:  "BINARY",
	2:  "INT",
	3:  "FLOAT",
	4:  "BOOL",
	5:  "DATE",
	6:  "DATETIME",
	7:  "GEO",
	8:  "UID",
	9:  "PASSWORD",
	10: "STRING",
}
var Posting_ValType_value = map[string]int32{
	"DEFAULT":  0,
	"BINARY":   1,
	"INT":      2,
	"FLOAT":    3,
	"BOOL":     4,
	"DATE":     5,
	"DATETIME": 6,
	"GEO":      7,
	"UID":      8,
	"PASSWORD": 9,
	"STRING":   10,
}

func (x Posting_ValType) String() string {
	return proto.EnumName(Posting_ValType_name, int32(x))
}
func (Posting_ValType) EnumDescriptor() ([]byte, []int) { return fileDescriptorTypes, []int{0, 0} }

type Posting_PostingType int32

const (
	Posting_REF        Posting_PostingType = 0
	Posting_VALUE      Posting_PostingType = 1
	Posting_VALUE_LANG Posting_PostingType = 2
)

var Posting_PostingType_name = map[int32]string{
	0: "REF",
	1: "VALUE",
	2: "VALUE_LANG",
}
var Posting_PostingType_value = map[string]int32{
	"REF":        0,
	"VALUE":      1,
	"VALUE_LANG": 2,
}

func (x Posting_PostingType) String() string {
	return proto.EnumName(Posting_PostingType_name, int32(x))
}
func (Posting_PostingType) EnumDescriptor() ([]byte, []int) { return fileDescriptorTypes, []int{0, 1} }

type Schema_Directive int32

const (
	Schema_NONE    Schema_Directive = 0
	Schema_INDEX   Schema_Directive = 1
	Schema_REVERSE Schema_Directive = 2
)

var Schema_Directive_name = map[int32]string{
	0: "NONE",
	1: "INDEX",
	2: "REVERSE",
}
var Schema_Directive_value = map[string]int32{
	"NONE":    0,
	"INDEX":   1,
	"REVERSE": 2,
}

func (x Schema_Directive) String() string {
	return proto.EnumName(Schema_Directive_name, int32(x))
}
func (Schema_Directive) EnumDescriptor() ([]byte, []int) { return fileDescriptorTypes, []int{2, 0} }

type Posting struct {
	Uid         uint64              `protobuf:"fixed64,1,opt,name=uid,proto3" json:"uid,omitempty"`
	Value       []byte              `protobuf:"bytes,2,opt,name=value,proto3" json:"value,omitempty"`
	ValType     Posting_ValType     `protobuf:"varint,3,opt,name=val_type,json=valType,proto3,enum=typesp.Posting_ValType" json:"val_type,omitempty"`
	PostingType Posting_PostingType `protobuf:"varint,4,opt,name=posting_type,json=postingType,proto3,enum=typesp.Posting_PostingType" json:"posting_type,omitempty"`
	Metadata    []byte              `protobuf:"bytes,5,opt,name=metadata,proto3" json:"metadata,omitempty"`
	Label       string              `protobuf:"bytes,6,opt,name=label,proto3" json:"label,omitempty"`
	Commit      uint64              `protobuf:"varint,7,opt,name=commit,proto3" json:"commit,omitempty"`
	Facets      []*facetsp.Facet    `protobuf:"bytes,8,rep,name=facets" json:"facets,omitempty"`
	// TODO: op is only used temporarily. See if we can remove it from here.
	Op uint32 `protobuf:"varint,12,opt,name=op,proto3" json:"op,omitempty"`
}

func (m *Posting) Reset()                    { *m = Posting{} }
func (m *Posting) String() string            { return proto.CompactTextString(m) }
func (*Posting) ProtoMessage()               {}
func (*Posting) Descriptor() ([]byte, []int) { return fileDescriptorTypes, []int{0} }

func (m *Posting) GetUid() uint64 {
	if m != nil {
		return m.Uid
	}
	return 0
}

func (m *Posting) GetValue() []byte {
	if m != nil {
		return m.Value
	}
	return nil
}

func (m *Posting) GetValType() Posting_ValType {
	if m != nil {
		return m.ValType
	}
	return Posting_DEFAULT
}

func (m *Posting) GetPostingType() Posting_PostingType {
	if m != nil {
		return m.PostingType
	}
	return Posting_REF
}

func (m *Posting) GetMetadata() []byte {
	if m != nil {
		return m.Metadata
	}
	return nil
}

func (m *Posting) GetLabel() string {
	if m != nil {
		return m.Label
	}
	return ""
}

func (m *Posting) GetCommit() uint64 {
	if m != nil {
		return m.Commit
	}
	return 0
}

func (m *Posting) GetFacets() []*facetsp.Facet {
	if m != nil {
		return m.Facets
	}
	return nil
}

func (m *Posting) GetOp() uint32 {
	if m != nil {
		return m.Op
	}
	return 0
}

type PostingList struct {
	Postings []*Posting `protobuf:"bytes,1,rep,name=postings" json:"postings,omitempty"`
	Checksum []byte     `protobuf:"bytes,2,opt,name=checksum,proto3" json:"checksum,omitempty"`
	Commit   uint64     `protobuf:"varint,3,opt,name=commit,proto3" json:"commit,omitempty"`
}

func (m *PostingList) Reset()                    { *m = PostingList{} }
func (m *PostingList) String() string            { return proto.CompactTextString(m) }
func (*PostingList) ProtoMessage()               {}
func (*PostingList) Descriptor() ([]byte, []int) { return fileDescriptorTypes, []int{1} }

func (m *PostingList) GetPostings() []*Posting {
	if m != nil {
		return m.Postings
	}
	return nil
}

func (m *PostingList) GetChecksum() []byte {
	if m != nil {
		return m.Checksum
	}
	return nil
}

func (m *PostingList) GetCommit() uint64 {
	if m != nil {
		return m.Commit
	}
	return 0
}

type Schema struct {
	ValueType uint32           `protobuf:"varint,1,opt,name=value_type,json=valueType,proto3" json:"value_type,omitempty"`
	Directive Schema_Directive `protobuf:"varint,2,opt,name=directive,proto3,enum=typesp.Schema_Directive" json:"directive,omitempty"`
	Tokenizer []string         `protobuf:"bytes,3,rep,name=tokenizer" json:"tokenizer,omitempty"`
}

func (m *Schema) Reset()                    { *m = Schema{} }
func (m *Schema) String() string            { return proto.CompactTextString(m) }
func (*Schema) ProtoMessage()               {}
func (*Schema) Descriptor() ([]byte, []int) { return fileDescriptorTypes, []int{2} }

func (m *Schema) GetValueType() uint32 {
	if m != nil {
		return m.ValueType
	}
	return 0
}

func (m *Schema) GetDirective() Schema_Directive {
	if m != nil {
		return m.Directive
	}
	return Schema_NONE
}

func (m *Schema) GetTokenizer() []string {
	if m != nil {
		return m.Tokenizer
	}
	return nil
}

func init() {
	proto.RegisterType((*Posting)(nil), "typesp.Posting")
	proto.RegisterType((*PostingList)(nil), "typesp.PostingList")
	proto.RegisterType((*Schema)(nil), "typesp.Schema")
	proto.RegisterEnum("typesp.Posting_ValType", Posting_ValType_name, Posting_ValType_value)
	proto.RegisterEnum("typesp.Posting_PostingType", Posting_PostingType_name, Posting_PostingType_value)
	proto.RegisterEnum("typesp.Schema_Directive", Schema_Directive_name, Schema_Directive_value)
}
func (m *Posting) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Posting) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if m.Uid != 0 {
		dAtA[i] = 0x9
		i++
		i = encodeFixed64Types(dAtA, i, uint64(m.Uid))
	}
	if len(m.Value) > 0 {
		dAtA[i] = 0x12
		i++
		i = encodeVarintTypes(dAtA, i, uint64(len(m.Value)))
		i += copy(dAtA[i:], m.Value)
	}
	if m.ValType != 0 {
		dAtA[i] = 0x18
		i++
		i = encodeVarintTypes(dAtA, i, uint64(m.ValType))
	}
	if m.PostingType != 0 {
		dAtA[i] = 0x20
		i++
		i = encodeVarintTypes(dAtA, i, uint64(m.PostingType))
	}
	if len(m.Metadata) > 0 {
		dAtA[i] = 0x2a
		i++
		i = encodeVarintTypes(dAtA, i, uint64(len(m.Metadata)))
		i += copy(dAtA[i:], m.Metadata)
	}
	if len(m.Label) > 0 {
		dAtA[i] = 0x32
		i++
		i = encodeVarintTypes(dAtA, i, uint64(len(m.Label)))
		i += copy(dAtA[i:], m.Label)
	}
	if m.Commit != 0 {
		dAtA[i] = 0x38
		i++
		i = encodeVarintTypes(dAtA, i, uint64(m.Commit))
	}
	if len(m.Facets) > 0 {
		for _, msg := range m.Facets {
			dAtA[i] = 0x42
			i++
			i = encodeVarintTypes(dAtA, i, uint64(msg.Size()))
			n, err := msg.MarshalTo(dAtA[i:])
			if err != nil {
				return 0, err
			}
			i += n
		}
	}
	if m.Op != 0 {
		dAtA[i] = 0x60
		i++
		i = encodeVarintTypes(dAtA, i, uint64(m.Op))
	}
	return i, nil
}

func (m *PostingList) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *PostingList) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if len(m.Postings) > 0 {
		for _, msg := range m.Postings {
			dAtA[i] = 0xa
			i++
			i = encodeVarintTypes(dAtA, i, uint64(msg.Size()))
			n, err := msg.MarshalTo(dAtA[i:])
			if err != nil {
				return 0, err
			}
			i += n
		}
	}
	if len(m.Checksum) > 0 {
		dAtA[i] = 0x12
		i++
		i = encodeVarintTypes(dAtA, i, uint64(len(m.Checksum)))
		i += copy(dAtA[i:], m.Checksum)
	}
	if m.Commit != 0 {
		dAtA[i] = 0x18
		i++
		i = encodeVarintTypes(dAtA, i, uint64(m.Commit))
	}
	return i, nil
}

func (m *Schema) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Schema) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if m.ValueType != 0 {
		dAtA[i] = 0x8
		i++
		i = encodeVarintTypes(dAtA, i, uint64(m.ValueType))
	}
	if m.Directive != 0 {
		dAtA[i] = 0x10
		i++
		i = encodeVarintTypes(dAtA, i, uint64(m.Directive))
	}
	if len(m.Tokenizer) > 0 {
		for _, s := range m.Tokenizer {
			dAtA[i] = 0x1a
			i++
			l = len(s)
			for l >= 1<<7 {
				dAtA[i] = uint8(uint64(l)&0x7f | 0x80)
				l >>= 7
				i++
			}
			dAtA[i] = uint8(l)
			i++
			i += copy(dAtA[i:], s)
		}
	}
	return i, nil
}

func encodeFixed64Types(dAtA []byte, offset int, v uint64) int {
	dAtA[offset] = uint8(v)
	dAtA[offset+1] = uint8(v >> 8)
	dAtA[offset+2] = uint8(v >> 16)
	dAtA[offset+3] = uint8(v >> 24)
	dAtA[offset+4] = uint8(v >> 32)
	dAtA[offset+5] = uint8(v >> 40)
	dAtA[offset+6] = uint8(v >> 48)
	dAtA[offset+7] = uint8(v >> 56)
	return offset + 8
}
func encodeFixed32Types(dAtA []byte, offset int, v uint32) int {
	dAtA[offset] = uint8(v)
	dAtA[offset+1] = uint8(v >> 8)
	dAtA[offset+2] = uint8(v >> 16)
	dAtA[offset+3] = uint8(v >> 24)
	return offset + 4
}
func encodeVarintTypes(dAtA []byte, offset int, v uint64) int {
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return offset + 1
}
func (m *Posting) Size() (n int) {
	var l int
	_ = l
	if m.Uid != 0 {
		n += 9
	}
	l = len(m.Value)
	if l > 0 {
		n += 1 + l + sovTypes(uint64(l))
	}
	if m.ValType != 0 {
		n += 1 + sovTypes(uint64(m.ValType))
	}
	if m.PostingType != 0 {
		n += 1 + sovTypes(uint64(m.PostingType))
	}
	l = len(m.Metadata)
	if l > 0 {
		n += 1 + l + sovTypes(uint64(l))
	}
	l = len(m.Label)
	if l > 0 {
		n += 1 + l + sovTypes(uint64(l))
	}
	if m.Commit != 0 {
		n += 1 + sovTypes(uint64(m.Commit))
	}
	if len(m.Facets) > 0 {
		for _, e := range m.Facets {
			l = e.Size()
			n += 1 + l + sovTypes(uint64(l))
		}
	}
	if m.Op != 0 {
		n += 1 + sovTypes(uint64(m.Op))
	}
	return n
}

func (m *PostingList) Size() (n int) {
	var l int
	_ = l
	if len(m.Postings) > 0 {
		for _, e := range m.Postings {
			l = e.Size()
			n += 1 + l + sovTypes(uint64(l))
		}
	}
	l = len(m.Checksum)
	if l > 0 {
		n += 1 + l + sovTypes(uint64(l))
	}
	if m.Commit != 0 {
		n += 1 + sovTypes(uint64(m.Commit))
	}
	return n
}

func (m *Schema) Size() (n int) {
	var l int
	_ = l
	if m.ValueType != 0 {
		n += 1 + sovTypes(uint64(m.ValueType))
	}
	if m.Directive != 0 {
		n += 1 + sovTypes(uint64(m.Directive))
	}
	if len(m.Tokenizer) > 0 {
		for _, s := range m.Tokenizer {
			l = len(s)
			n += 1 + l + sovTypes(uint64(l))
		}
	}
	return n
}

func sovTypes(x uint64) (n int) {
	for {
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	return n
}
func sozTypes(x uint64) (n int) {
	return sovTypes(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *Posting) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowTypes
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Posting: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Posting: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 1 {
				return fmt.Errorf("proto: wrong wireType = %d for field Uid", wireType)
			}
			m.Uid = 0
			if (iNdEx + 8) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += 8
			m.Uid = uint64(dAtA[iNdEx-8])
			m.Uid |= uint64(dAtA[iNdEx-7]) << 8
			m.Uid |= uint64(dAtA[iNdEx-6]) << 16
			m.Uid |= uint64(dAtA[iNdEx-5]) << 24
			m.Uid |= uint64(dAtA[iNdEx-4]) << 32
			m.Uid |= uint64(dAtA[iNdEx-3]) << 40
			m.Uid |= uint64(dAtA[iNdEx-2]) << 48
			m.Uid |= uint64(dAtA[iNdEx-1]) << 56
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Value", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthTypes
			}
			postIndex := iNdEx + byteLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Value = append(m.Value[:0], dAtA[iNdEx:postIndex]...)
			if m.Value == nil {
				m.Value = []byte{}
			}
			iNdEx = postIndex
		case 3:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field ValType", wireType)
			}
			m.ValType = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.ValType |= (Posting_ValType(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 4:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field PostingType", wireType)
			}
			m.PostingType = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.PostingType |= (Posting_PostingType(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 5:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Metadata", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthTypes
			}
			postIndex := iNdEx + byteLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Metadata = append(m.Metadata[:0], dAtA[iNdEx:postIndex]...)
			if m.Metadata == nil {
				m.Metadata = []byte{}
			}
			iNdEx = postIndex
		case 6:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Label", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthTypes
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Label = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 7:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Commit", wireType)
			}
			m.Commit = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Commit |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 8:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Facets", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthTypes
			}
			postIndex := iNdEx + msglen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Facets = append(m.Facets, &facetsp.Facet{})
			if err := m.Facets[len(m.Facets)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 12:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Op", wireType)
			}
			m.Op = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Op |= (uint32(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		default:
			iNdEx = preIndex
			skippy, err := skipTypes(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthTypes
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *PostingList) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowTypes
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: PostingList: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: PostingList: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Postings", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthTypes
			}
			postIndex := iNdEx + msglen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Postings = append(m.Postings, &Posting{})
			if err := m.Postings[len(m.Postings)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Checksum", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthTypes
			}
			postIndex := iNdEx + byteLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Checksum = append(m.Checksum[:0], dAtA[iNdEx:postIndex]...)
			if m.Checksum == nil {
				m.Checksum = []byte{}
			}
			iNdEx = postIndex
		case 3:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Commit", wireType)
			}
			m.Commit = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Commit |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		default:
			iNdEx = preIndex
			skippy, err := skipTypes(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthTypes
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *Schema) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowTypes
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Schema: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Schema: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field ValueType", wireType)
			}
			m.ValueType = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.ValueType |= (uint32(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Directive", wireType)
			}
			m.Directive = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Directive |= (Schema_Directive(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 3:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Tokenizer", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthTypes
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Tokenizer = append(m.Tokenizer, string(dAtA[iNdEx:postIndex]))
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipTypes(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthTypes
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func skipTypes(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowTypes
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowTypes
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
			return iNdEx, nil
		case 1:
			iNdEx += 8
			return iNdEx, nil
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowTypes
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			iNdEx += length
			if length < 0 {
				return 0, ErrInvalidLengthTypes
			}
			return iNdEx, nil
		case 3:
			for {
				var innerWire uint64
				var start int = iNdEx
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, ErrIntOverflowTypes
					}
					if iNdEx >= l {
						return 0, io.ErrUnexpectedEOF
					}
					b := dAtA[iNdEx]
					iNdEx++
					innerWire |= (uint64(b) & 0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				innerWireType := int(innerWire & 0x7)
				if innerWireType == 4 {
					break
				}
				next, err := skipTypes(dAtA[start:])
				if err != nil {
					return 0, err
				}
				iNdEx = start + next
			}
			return iNdEx, nil
		case 4:
			return iNdEx, nil
		case 5:
			iNdEx += 4
			return iNdEx, nil
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
	}
	panic("unreachable")
}

var (
	ErrInvalidLengthTypes = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowTypes   = fmt.Errorf("proto: integer overflow")
)

func init() { proto.RegisterFile("protos/typesp/types.proto", fileDescriptorTypes) }

var fileDescriptorTypes = []byte{
	// 577 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x09, 0x6e, 0x88, 0x02, 0xff, 0x5c, 0x53, 0xdd, 0x6e, 0x9b, 0x4c,
	0x10, 0xcd, 0x82, 0xcd, 0xcf, 0xd8, 0x9f, 0xbf, 0xd5, 0xaa, 0x6a, 0x69, 0xd2, 0x5a, 0x88, 0x8b,
	0x8a, 0xaa, 0x0a, 0x51, 0x5d, 0xa9, 0x37, 0x95, 0x2a, 0x11, 0x81, 0x23, 0x24, 0x0a, 0xd1, 0x9a,
	0xa4, 0x3f, 0x37, 0x11, 0xc1, 0x34, 0x46, 0x31, 0x01, 0x19, 0x6c, 0x29, 0xbd, 0xee, 0x43, 0xf4,
	0x21, 0xfa, 0x20, 0xbd, 0xec, 0x23, 0xb4, 0xee, 0x8b, 0x54, 0xcb, 0x12, 0xdb, 0xcd, 0xd5, 0x9c,
	0x33, 0x33, 0x67, 0x77, 0x38, 0x3b, 0xc0, 0xe3, 0x72, 0x51, 0xd4, 0x45, 0x75, 0x54, 0xdf, 0x96,
	0x69, 0x55, 0xf2, 0x60, 0x35, 0x39, 0x22, 0xf1, 0xdc, 0xfe, 0x41, 0xdb, 0xf2, 0x39, 0x4e, 0xd2,
	0xba, 0x2a, 0xdb, 0xc8, 0x9b, 0x8c, 0xdf, 0x22, 0xc8, 0xa7, 0x45, 0x55, 0x67, 0x37, 0x57, 0x04,
	0x83, 0xb8, 0xcc, 0xa6, 0x1a, 0xd2, 0x91, 0x29, 0x51, 0x06, 0xc9, 0x03, 0xe8, 0xae, 0xe2, 0xf9,
	0x32, 0xd5, 0x04, 0x1d, 0x99, 0x7d, 0xca, 0x09, 0x19, 0x81, 0xb2, 0x8a, 0xe7, 0x17, 0xec, 0x78,
	0x4d, 0xd4, 0x91, 0x39, 0x18, 0x3d, 0xb2, 0xf8, 0x5d, 0x56, 0x7b, 0x94, 0x75, 0x1e, 0xcf, 0xa3,
	0xdb, 0x32, 0xa5, 0xf2, 0x8a, 0x03, 0xf2, 0x16, 0xfa, 0x25, 0xaf, 0x71, 0x5d, 0xa7, 0xd1, 0x1d,
	0xdc, 0xd7, 0xb5, 0xb1, 0xd1, 0xf6, 0xca, 0x2d, 0x21, 0xfb, 0xa0, 0xe4, 0x69, 0x1d, 0x4f, 0xe3,
	0x3a, 0xd6, 0xba, 0xcd, 0x30, 0x1b, 0xce, 0xa6, 0x9c, 0xc7, 0x97, 0xe9, 0x5c, 0x93, 0x74, 0x64,
	0xaa, 0x94, 0x13, 0xf2, 0x10, 0xa4, 0xa4, 0xc8, 0xf3, 0xac, 0xd6, 0x64, 0x1d, 0x99, 0x1d, 0xda,
	0x32, 0xf2, 0x0c, 0x24, 0xee, 0x80, 0xa6, 0xe8, 0xa2, 0xd9, 0x1b, 0x0d, 0xac, 0xd6, 0x18, 0x6b,
	0xcc, 0x22, 0x6d, 0xab, 0x64, 0x00, 0x42, 0x51, 0x6a, 0x7d, 0x1d, 0x99, 0xff, 0x51, 0xa1, 0x28,
	0x8d, 0xaf, 0x08, 0xe4, 0xf6, 0xb3, 0x48, 0x0f, 0x64, 0xc7, 0x1d, 0xdb, 0x67, 0x7e, 0x84, 0xf7,
	0x08, 0x80, 0x74, 0xec, 0x05, 0x36, 0xfd, 0x88, 0x11, 0x91, 0x41, 0xf4, 0x82, 0x08, 0x0b, 0x44,
	0x85, 0xee, 0xd8, 0x0f, 0xed, 0x08, 0x8b, 0x44, 0x81, 0xce, 0x71, 0x18, 0xfa, 0xb8, 0xc3, 0x90,
	0x63, 0x47, 0x2e, 0xee, 0x92, 0x3e, 0x28, 0x0c, 0x45, 0xde, 0x3b, 0x17, 0x4b, 0x4c, 0x75, 0xe2,
	0x86, 0x58, 0x66, 0xe0, 0xcc, 0x73, 0xb0, 0xc2, 0xea, 0xa7, 0xf6, 0x64, 0xf2, 0x3e, 0xa4, 0x0e,
	0x56, 0xd9, 0x0d, 0x93, 0x88, 0x7a, 0xc1, 0x09, 0x06, 0xe3, 0x25, 0xf4, 0x76, 0x4c, 0x62, 0x0a,
	0xea, 0x8e, 0xf1, 0x1e, 0xbb, 0xf0, 0xdc, 0xf6, 0xcf, 0x5c, 0x8c, 0xc8, 0x00, 0xa0, 0x81, 0x17,
	0xbe, 0x1d, 0x9c, 0x60, 0xc1, 0xb8, 0xd9, 0x48, 0xfc, 0xac, 0xaa, 0xc9, 0x0b, 0x50, 0x5a, 0x67,
	0x2b, 0x0d, 0x35, 0x16, 0xfc, 0x7f, 0xef, 0x19, 0xe8, 0xa6, 0x81, 0xf9, 0x9e, 0xcc, 0xd2, 0xe4,
	0xba, 0x5a, 0xe6, 0xed, 0x12, 0x6c, 0xf8, 0x8e, 0xc3, 0xe2, 0xae, 0xc3, 0xc6, 0x77, 0x04, 0xd2,
	0x24, 0x99, 0xa5, 0x79, 0x4c, 0x9e, 0x02, 0x34, 0x3b, 0xc3, 0x1f, 0x1d, 0x35, 0x66, 0xaa, 0x4d,
	0xa6, 0x99, 0xfe, 0x35, 0xa8, 0xd3, 0x6c, 0x91, 0x26, 0x75, 0xb6, 0xe2, 0x3b, 0x36, 0x18, 0x69,
	0x77, 0xb3, 0xf0, 0x13, 0x2c, 0xe7, 0xae, 0x4e, 0xb7, 0xad, 0xe4, 0x09, 0xa8, 0x75, 0x71, 0x9d,
	0xde, 0x64, 0x5f, 0xd2, 0x85, 0x26, 0xea, 0xa2, 0xa9, 0xd2, 0x6d, 0xc2, 0x38, 0x04, 0x75, 0xa3,
	0x62, 0x9e, 0x07, 0x61, 0xe0, 0x72, 0x87, 0xbc, 0xc0, 0x71, 0x3f, 0x60, 0xc4, 0xde, 0x8f, 0xba,
	0xe7, 0x2e, 0x9d, 0xb8, 0x58, 0x38, 0x7e, 0xf3, 0x63, 0x3d, 0x44, 0x3f, 0xd7, 0x43, 0xf4, 0x6b,
	0x3d, 0x44, 0xdf, 0xfe, 0x0c, 0xf7, 0x3e, 0x3d, 0xbf, 0xca, 0xea, 0xd9, 0xf2, 0xd2, 0x4a, 0x8a,
	0xfc, 0x68, 0x7a, 0xb5, 0x88, 0xcb, 0xd9, 0x61, 0x56, 0xb4, 0xe8, 0xe8, 0x9f, 0x1f, 0xee, 0x52,
	0x6a, 0xe8, 0xab, 0xbf, 0x01, 0x00, 0x00, 0xff, 0xff, 0x8b, 0xb5, 0xaa, 0x8a, 0x88, 0x03, 0x00,
	0x00,
}
