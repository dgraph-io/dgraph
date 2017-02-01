// Code generated by protoc-gen-gogo.
// source: types/types.proto
// DO NOT EDIT!

/*
	Package types is a generated protocol buffer package.

	It is generated from these files:
		types/types.proto

	It has these top-level messages:
		Posting
		PostingList
*/
package types

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import dprotoc "github.com/dgraph-io/dgraph/dprotoc"

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

type Posting struct {
	Uid     uint64           `protobuf:"fixed64,1,opt,name=uid,proto3" json:"uid,omitempty"`
	Value   []byte           `protobuf:"bytes,2,opt,name=value,proto3" json:"value,omitempty"`
	ValType dprotoc.ValType  `protobuf:"varint,3,opt,name=val_type,json=valType,proto3,enum=dprotoc.ValType" json:"val_type,omitempty"`
	Lang    string           `protobuf:"bytes,4,opt,name=lang,proto3" json:"lang,omitempty"`
	Label   string           `protobuf:"bytes,5,opt,name=label,proto3" json:"label,omitempty"`
	Commit  uint64           `protobuf:"varint,6,opt,name=commit,proto3" json:"commit,omitempty"`
	Facets  []*dprotoc.Facet `protobuf:"bytes,7,rep,name=facets" json:"facets,omitempty"`
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

func (m *Posting) GetValType() dprotoc.ValType {
	if m != nil {
		return m.ValType
	}
	return dprotoc.ValType_STRING
}

func (m *Posting) GetLang() string {
	if m != nil {
		return m.Lang
	}
	return ""
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

func (m *Posting) GetFacets() []*dprotoc.Facet {
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

func init() {
	proto.RegisterType((*Posting)(nil), "types.Posting")
	proto.RegisterType((*PostingList)(nil), "types.PostingList")
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
	if len(m.Lang) > 0 {
		dAtA[i] = 0x22
		i++
		i = encodeVarintTypes(dAtA, i, uint64(len(m.Lang)))
		i += copy(dAtA[i:], m.Lang)
	}
	if len(m.Label) > 0 {
		dAtA[i] = 0x2a
		i++
		i = encodeVarintTypes(dAtA, i, uint64(len(m.Label)))
		i += copy(dAtA[i:], m.Label)
	}
	if m.Commit != 0 {
		dAtA[i] = 0x30
		i++
		i = encodeVarintTypes(dAtA, i, uint64(m.Commit))
	}
	if len(m.Facets) > 0 {
		for _, msg := range m.Facets {
			dAtA[i] = 0x3a
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
	l = len(m.Lang)
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
				m.ValType |= (dprotoc.ValType(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 4:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Lang", wireType)
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
			m.Lang = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 5:
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
		case 6:
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
		case 7:
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
			m.Facets = append(m.Facets, &dprotoc.Facet{})
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

func init() { proto.RegisterFile("types/types.proto", fileDescriptorTypes) }

var fileDescriptorTypes = []byte{
	// 292 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x09, 0x6e, 0x88, 0x02, 0xff, 0x4c, 0x90, 0xc1, 0x4a, 0x33, 0x31,
	0x14, 0x85, 0xff, 0xdb, 0x69, 0xa7, 0xfd, 0x6f, 0x6b, 0xa9, 0xa1, 0x48, 0xe8, 0x62, 0x08, 0x5d,
	0x48, 0x50, 0x68, 0xa1, 0xbe, 0x81, 0x0b, 0x57, 0x2e, 0x24, 0x88, 0x5b, 0x49, 0xa7, 0xb1, 0x0e,
	0x66, 0x9a, 0xe0, 0xa4, 0x85, 0xbe, 0x89, 0x8f, 0xe4, 0xd2, 0xa5, 0x4b, 0x19, 0x5f, 0x44, 0x26,
	0x09, 0x43, 0x37, 0xe1, 0x7c, 0xf7, 0x84, 0x93, 0x73, 0x83, 0xe7, 0xee, 0x68, 0x55, 0xb5, 0xf4,
	0xe7, 0xc2, 0xbe, 0x1b, 0x67, 0x48, 0xcf, 0xc3, 0x6c, 0xba, 0xf1, 0x98, 0x2f, 0x5f, 0x64, 0xae,
	0x5c, 0x34, 0xe7, 0xdf, 0x80, 0xfd, 0x07, 0x53, 0xb9, 0x62, 0xb7, 0x25, 0x13, 0x4c, 0xf6, 0xc5,
	0x86, 0x02, 0x03, 0x9e, 0x8a, 0x46, 0x92, 0x29, 0xf6, 0x0e, 0x52, 0xef, 0x15, 0xed, 0x30, 0xe0,
	0x23, 0x11, 0x80, 0x5c, 0xe3, 0xe0, 0x20, 0xf5, 0x73, 0x13, 0x4b, 0x13, 0x06, 0x7c, 0xbc, 0x9a,
	0x2c, 0x62, 0xf8, 0xe2, 0x49, 0xea, 0xc7, 0xa3, 0x55, 0xa2, 0x7f, 0x08, 0x82, 0x10, 0xec, 0x6a,
	0xb9, 0xdb, 0xd2, 0x2e, 0x03, 0xfe, 0x5f, 0x78, 0xdd, 0xc4, 0x6a, 0xb9, 0x56, 0x9a, 0xf6, 0xfc,
	0x30, 0x00, 0xb9, 0xc0, 0x34, 0x37, 0x65, 0x59, 0x38, 0x9a, 0x32, 0xe0, 0x5d, 0x11, 0x89, 0x5c,
	0x62, 0x1a, 0x2a, 0xd3, 0x3e, 0x4b, 0xf8, 0x70, 0x35, 0x6e, 0x1f, 0xbb, 0x6b, 0xc6, 0x22, 0xba,
	0x64, 0x8c, 0x1d, 0x63, 0xe9, 0x88, 0x01, 0x3f, 0x13, 0x1d, 0x63, 0xe7, 0x25, 0x0e, 0xe3, 0x66,
	0xf7, 0x45, 0xe5, 0xc8, 0x15, 0x0e, 0x6c, 0xc0, 0x8a, 0x42, 0x0c, 0x0a, 0xdf, 0x14, 0x6f, 0x89,
	0xd6, 0x27, 0x33, 0x1c, 0xe4, 0xaf, 0x2a, 0x7f, 0xab, 0xf6, 0x65, 0x5c, 0xbd, 0xe5, 0x93, 0x9a,
	0xc9, 0x69, 0xcd, 0xdb, 0xc9, 0x67, 0x9d, 0xc1, 0x57, 0x9d, 0xc1, 0x4f, 0x9d, 0xc1, 0xc7, 0x6f,
	0xf6, 0x6f, 0x9d, 0xfa, 0x9a, 0x37, 0x7f, 0x01, 0x00, 0x00, 0xff, 0xff, 0x0e, 0xba, 0xfe, 0x86,
	0x94, 0x01, 0x00, 0x00,
}
