// Code generated by protoc-gen-gogo.
// source: worker/payload.proto
// DO NOT EDIT!

/*
	Package worker is a generated protocol buffer package.

	It is generated from these files:
		worker/payload.proto

	It has these top-level messages:
		Payload
		BackupPayload
*/
package worker

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import task "github.com/dgraph-io/dgraph/task"

import (
	context "golang.org/x/net/context"
	grpc "google.golang.org/grpc"
)

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

type BackupPayload_Status int32

const (
	BackupPayload_NONE      BackupPayload_Status = 0
	BackupPayload_SUCCESS   BackupPayload_Status = 1
	BackupPayload_DUPLICATE BackupPayload_Status = 2
	BackupPayload_FAILED    BackupPayload_Status = 3
)

var BackupPayload_Status_name = map[int32]string{
	0: "NONE",
	1: "SUCCESS",
	2: "DUPLICATE",
	3: "FAILED",
}
var BackupPayload_Status_value = map[string]int32{
	"NONE":      0,
	"SUCCESS":   1,
	"DUPLICATE": 2,
	"FAILED":    3,
}

func (x BackupPayload_Status) String() string {
	return proto.EnumName(BackupPayload_Status_name, int32(x))
}
func (BackupPayload_Status) EnumDescriptor() ([]byte, []int) {
	return fileDescriptorPayload, []int{1, 0}
}

type Payload struct {
	Data []byte `protobuf:"bytes,1,opt,name=Data,json=data,proto3" json:"Data,omitempty"`
}

func (m *Payload) Reset()                    { *m = Payload{} }
func (m *Payload) String() string            { return proto.CompactTextString(m) }
func (*Payload) ProtoMessage()               {}
func (*Payload) Descriptor() ([]byte, []int) { return fileDescriptorPayload, []int{0} }

// BackupPayload is used both as a request and a response.
// When used in request, groups represents the list of groups that need to be backed up.
// When used in response, groups represent the list of groups that were backed up.
type BackupPayload struct {
	ReqId   uint64               `protobuf:"varint,1,opt,name=req_id,json=reqId,proto3" json:"req_id,omitempty"`
	GroupId uint32               `protobuf:"varint,2,opt,name=group_id,json=groupId,proto3" json:"group_id,omitempty"`
	Status  BackupPayload_Status `protobuf:"varint,3,opt,name=status,proto3,enum=worker.BackupPayload_Status" json:"status,omitempty"`
}

func (m *BackupPayload) Reset()                    { *m = BackupPayload{} }
func (m *BackupPayload) String() string            { return proto.CompactTextString(m) }
func (*BackupPayload) ProtoMessage()               {}
func (*BackupPayload) Descriptor() ([]byte, []int) { return fileDescriptorPayload, []int{1} }

func init() {
	proto.RegisterType((*Payload)(nil), "worker.Payload")
	proto.RegisterType((*BackupPayload)(nil), "worker.BackupPayload")
	proto.RegisterEnum("worker.BackupPayload_Status", BackupPayload_Status_name, BackupPayload_Status_value)
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion3

// Client API for Worker service

type WorkerClient interface {
	// Connection testing RPC.
	Echo(ctx context.Context, in *Payload, opts ...grpc.CallOption) (*Payload, error)
	// Data serving RPCs.
	AssignUids(ctx context.Context, in *task.Num, opts ...grpc.CallOption) (*task.List, error)
	Mutate(ctx context.Context, in *task.Mutations, opts ...grpc.CallOption) (*Payload, error)
	ServeTask(ctx context.Context, in *task.Query, opts ...grpc.CallOption) (*task.Result, error)
	PredicateData(ctx context.Context, in *Payload, opts ...grpc.CallOption) (Worker_PredicateDataClient, error)
	Sort(ctx context.Context, in *task.Sort, opts ...grpc.CallOption) (*task.SortResult, error)
	// RAFT serving RPCs.
	RaftMessage(ctx context.Context, in *Payload, opts ...grpc.CallOption) (*Payload, error)
	JoinCluster(ctx context.Context, in *task.RaftContext, opts ...grpc.CallOption) (*Payload, error)
	UpdateMembership(ctx context.Context, in *task.MembershipUpdate, opts ...grpc.CallOption) (*task.MembershipUpdate, error)
	Backup(ctx context.Context, in *BackupPayload, opts ...grpc.CallOption) (*BackupPayload, error)
}

type workerClient struct {
	cc *grpc.ClientConn
}

func NewWorkerClient(cc *grpc.ClientConn) WorkerClient {
	return &workerClient{cc}
}

func (c *workerClient) Echo(ctx context.Context, in *Payload, opts ...grpc.CallOption) (*Payload, error) {
	out := new(Payload)
	err := grpc.Invoke(ctx, "/worker.Worker/Echo", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *workerClient) AssignUids(ctx context.Context, in *task.Num, opts ...grpc.CallOption) (*task.List, error) {
	out := new(task.List)
	err := grpc.Invoke(ctx, "/worker.Worker/AssignUids", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *workerClient) Mutate(ctx context.Context, in *task.Mutations, opts ...grpc.CallOption) (*Payload, error) {
	out := new(Payload)
	err := grpc.Invoke(ctx, "/worker.Worker/Mutate", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *workerClient) ServeTask(ctx context.Context, in *task.Query, opts ...grpc.CallOption) (*task.Result, error) {
	out := new(task.Result)
	err := grpc.Invoke(ctx, "/worker.Worker/ServeTask", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *workerClient) PredicateData(ctx context.Context, in *Payload, opts ...grpc.CallOption) (Worker_PredicateDataClient, error) {
	stream, err := grpc.NewClientStream(ctx, &_Worker_serviceDesc.Streams[0], c.cc, "/worker.Worker/PredicateData", opts...)
	if err != nil {
		return nil, err
	}
	x := &workerPredicateDataClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Worker_PredicateDataClient interface {
	Recv() (*Payload, error)
	grpc.ClientStream
}

type workerPredicateDataClient struct {
	grpc.ClientStream
}

func (x *workerPredicateDataClient) Recv() (*Payload, error) {
	m := new(Payload)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *workerClient) Sort(ctx context.Context, in *task.Sort, opts ...grpc.CallOption) (*task.SortResult, error) {
	out := new(task.SortResult)
	err := grpc.Invoke(ctx, "/worker.Worker/Sort", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *workerClient) RaftMessage(ctx context.Context, in *Payload, opts ...grpc.CallOption) (*Payload, error) {
	out := new(Payload)
	err := grpc.Invoke(ctx, "/worker.Worker/RaftMessage", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *workerClient) JoinCluster(ctx context.Context, in *task.RaftContext, opts ...grpc.CallOption) (*Payload, error) {
	out := new(Payload)
	err := grpc.Invoke(ctx, "/worker.Worker/JoinCluster", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *workerClient) UpdateMembership(ctx context.Context, in *task.MembershipUpdate, opts ...grpc.CallOption) (*task.MembershipUpdate, error) {
	out := new(task.MembershipUpdate)
	err := grpc.Invoke(ctx, "/worker.Worker/UpdateMembership", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *workerClient) Backup(ctx context.Context, in *BackupPayload, opts ...grpc.CallOption) (*BackupPayload, error) {
	out := new(BackupPayload)
	err := grpc.Invoke(ctx, "/worker.Worker/Backup", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Server API for Worker service

type WorkerServer interface {
	// Connection testing RPC.
	Echo(context.Context, *Payload) (*Payload, error)
	// Data serving RPCs.
	AssignUids(context.Context, *task.Num) (*task.List, error)
	Mutate(context.Context, *task.Mutations) (*Payload, error)
	ServeTask(context.Context, *task.Query) (*task.Result, error)
	PredicateData(*Payload, Worker_PredicateDataServer) error
	Sort(context.Context, *task.Sort) (*task.SortResult, error)
	// RAFT serving RPCs.
	RaftMessage(context.Context, *Payload) (*Payload, error)
	JoinCluster(context.Context, *task.RaftContext) (*Payload, error)
	UpdateMembership(context.Context, *task.MembershipUpdate) (*task.MembershipUpdate, error)
	Backup(context.Context, *BackupPayload) (*BackupPayload, error)
}

func RegisterWorkerServer(s *grpc.Server, srv WorkerServer) {
	s.RegisterService(&_Worker_serviceDesc, srv)
}

func _Worker_Echo_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Payload)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WorkerServer).Echo(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/worker.Worker/Echo",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WorkerServer).Echo(ctx, req.(*Payload))
	}
	return interceptor(ctx, in, info, handler)
}

func _Worker_AssignUids_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(task.Num)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WorkerServer).AssignUids(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/worker.Worker/AssignUids",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WorkerServer).AssignUids(ctx, req.(*task.Num))
	}
	return interceptor(ctx, in, info, handler)
}

func _Worker_Mutate_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(task.Mutations)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WorkerServer).Mutate(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/worker.Worker/Mutate",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WorkerServer).Mutate(ctx, req.(*task.Mutations))
	}
	return interceptor(ctx, in, info, handler)
}

func _Worker_ServeTask_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(task.Query)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WorkerServer).ServeTask(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/worker.Worker/ServeTask",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WorkerServer).ServeTask(ctx, req.(*task.Query))
	}
	return interceptor(ctx, in, info, handler)
}

func _Worker_PredicateData_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(Payload)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(WorkerServer).PredicateData(m, &workerPredicateDataServer{stream})
}

type Worker_PredicateDataServer interface {
	Send(*Payload) error
	grpc.ServerStream
}

type workerPredicateDataServer struct {
	grpc.ServerStream
}

func (x *workerPredicateDataServer) Send(m *Payload) error {
	return x.ServerStream.SendMsg(m)
}

func _Worker_Sort_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(task.Sort)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WorkerServer).Sort(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/worker.Worker/Sort",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WorkerServer).Sort(ctx, req.(*task.Sort))
	}
	return interceptor(ctx, in, info, handler)
}

func _Worker_RaftMessage_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Payload)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WorkerServer).RaftMessage(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/worker.Worker/RaftMessage",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WorkerServer).RaftMessage(ctx, req.(*Payload))
	}
	return interceptor(ctx, in, info, handler)
}

func _Worker_JoinCluster_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(task.RaftContext)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WorkerServer).JoinCluster(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/worker.Worker/JoinCluster",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WorkerServer).JoinCluster(ctx, req.(*task.RaftContext))
	}
	return interceptor(ctx, in, info, handler)
}

func _Worker_UpdateMembership_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(task.MembershipUpdate)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WorkerServer).UpdateMembership(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/worker.Worker/UpdateMembership",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WorkerServer).UpdateMembership(ctx, req.(*task.MembershipUpdate))
	}
	return interceptor(ctx, in, info, handler)
}

func _Worker_Backup_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(BackupPayload)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(WorkerServer).Backup(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/worker.Worker/Backup",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(WorkerServer).Backup(ctx, req.(*BackupPayload))
	}
	return interceptor(ctx, in, info, handler)
}

var _Worker_serviceDesc = grpc.ServiceDesc{
	ServiceName: "worker.Worker",
	HandlerType: (*WorkerServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Echo",
			Handler:    _Worker_Echo_Handler,
		},
		{
			MethodName: "AssignUids",
			Handler:    _Worker_AssignUids_Handler,
		},
		{
			MethodName: "Mutate",
			Handler:    _Worker_Mutate_Handler,
		},
		{
			MethodName: "ServeTask",
			Handler:    _Worker_ServeTask_Handler,
		},
		{
			MethodName: "Sort",
			Handler:    _Worker_Sort_Handler,
		},
		{
			MethodName: "RaftMessage",
			Handler:    _Worker_RaftMessage_Handler,
		},
		{
			MethodName: "JoinCluster",
			Handler:    _Worker_JoinCluster_Handler,
		},
		{
			MethodName: "UpdateMembership",
			Handler:    _Worker_UpdateMembership_Handler,
		},
		{
			MethodName: "Backup",
			Handler:    _Worker_Backup_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "PredicateData",
			Handler:       _Worker_PredicateData_Handler,
			ServerStreams: true,
		},
	},
	Metadata: fileDescriptorPayload,
}

func (m *Payload) Marshal() (data []byte, err error) {
	size := m.Size()
	data = make([]byte, size)
	n, err := m.MarshalTo(data)
	if err != nil {
		return nil, err
	}
	return data[:n], nil
}

func (m *Payload) MarshalTo(data []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if len(m.Data) > 0 {
		data[i] = 0xa
		i++
		i = encodeVarintPayload(data, i, uint64(len(m.Data)))
		i += copy(data[i:], m.Data)
	}
	return i, nil
}

func (m *BackupPayload) Marshal() (data []byte, err error) {
	size := m.Size()
	data = make([]byte, size)
	n, err := m.MarshalTo(data)
	if err != nil {
		return nil, err
	}
	return data[:n], nil
}

func (m *BackupPayload) MarshalTo(data []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if m.ReqId != 0 {
		data[i] = 0x8
		i++
		i = encodeVarintPayload(data, i, uint64(m.ReqId))
	}
	if m.GroupId != 0 {
		data[i] = 0x10
		i++
		i = encodeVarintPayload(data, i, uint64(m.GroupId))
	}
	if m.Status != 0 {
		data[i] = 0x18
		i++
		i = encodeVarintPayload(data, i, uint64(m.Status))
	}
	return i, nil
}

func encodeFixed64Payload(data []byte, offset int, v uint64) int {
	data[offset] = uint8(v)
	data[offset+1] = uint8(v >> 8)
	data[offset+2] = uint8(v >> 16)
	data[offset+3] = uint8(v >> 24)
	data[offset+4] = uint8(v >> 32)
	data[offset+5] = uint8(v >> 40)
	data[offset+6] = uint8(v >> 48)
	data[offset+7] = uint8(v >> 56)
	return offset + 8
}
func encodeFixed32Payload(data []byte, offset int, v uint32) int {
	data[offset] = uint8(v)
	data[offset+1] = uint8(v >> 8)
	data[offset+2] = uint8(v >> 16)
	data[offset+3] = uint8(v >> 24)
	return offset + 4
}
func encodeVarintPayload(data []byte, offset int, v uint64) int {
	for v >= 1<<7 {
		data[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	data[offset] = uint8(v)
	return offset + 1
}
func (m *Payload) Size() (n int) {
	var l int
	_ = l
	l = len(m.Data)
	if l > 0 {
		n += 1 + l + sovPayload(uint64(l))
	}
	return n
}

func (m *BackupPayload) Size() (n int) {
	var l int
	_ = l
	if m.ReqId != 0 {
		n += 1 + sovPayload(uint64(m.ReqId))
	}
	if m.GroupId != 0 {
		n += 1 + sovPayload(uint64(m.GroupId))
	}
	if m.Status != 0 {
		n += 1 + sovPayload(uint64(m.Status))
	}
	return n
}

func sovPayload(x uint64) (n int) {
	for {
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	return n
}
func sozPayload(x uint64) (n int) {
	return sovPayload(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *Payload) Unmarshal(data []byte) error {
	l := len(data)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowPayload
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := data[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Payload: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Payload: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Data", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowPayload
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := data[iNdEx]
				iNdEx++
				byteLen |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthPayload
			}
			postIndex := iNdEx + byteLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Data = append(m.Data[:0], data[iNdEx:postIndex]...)
			if m.Data == nil {
				m.Data = []byte{}
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipPayload(data[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthPayload
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
func (m *BackupPayload) Unmarshal(data []byte) error {
	l := len(data)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowPayload
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := data[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: BackupPayload: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: BackupPayload: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field ReqId", wireType)
			}
			m.ReqId = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowPayload
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := data[iNdEx]
				iNdEx++
				m.ReqId |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field GroupId", wireType)
			}
			m.GroupId = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowPayload
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := data[iNdEx]
				iNdEx++
				m.GroupId |= (uint32(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 3:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Status", wireType)
			}
			m.Status = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowPayload
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := data[iNdEx]
				iNdEx++
				m.Status |= (BackupPayload_Status(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		default:
			iNdEx = preIndex
			skippy, err := skipPayload(data[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthPayload
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
func skipPayload(data []byte) (n int, err error) {
	l := len(data)
	iNdEx := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowPayload
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := data[iNdEx]
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
					return 0, ErrIntOverflowPayload
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if data[iNdEx-1] < 0x80 {
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
					return 0, ErrIntOverflowPayload
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := data[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			iNdEx += length
			if length < 0 {
				return 0, ErrInvalidLengthPayload
			}
			return iNdEx, nil
		case 3:
			for {
				var innerWire uint64
				var start int = iNdEx
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, ErrIntOverflowPayload
					}
					if iNdEx >= l {
						return 0, io.ErrUnexpectedEOF
					}
					b := data[iNdEx]
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
				next, err := skipPayload(data[start:])
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
	ErrInvalidLengthPayload = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowPayload   = fmt.Errorf("proto: integer overflow")
)

func init() { proto.RegisterFile("worker/payload.proto", fileDescriptorPayload) }

var fileDescriptorPayload = []byte{
	// 492 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x09, 0x6e, 0x88, 0x02, 0xff, 0x8c, 0x52, 0xdd, 0x6e, 0xd3, 0x4c,
	0x10, 0xf5, 0x36, 0xfe, 0x9c, 0x66, 0xd2, 0x7c, 0x35, 0x23, 0x8a, 0x8a, 0x05, 0x51, 0x64, 0x09,
	0x14, 0xf1, 0xe3, 0x40, 0x0b, 0x12, 0xe2, 0x2e, 0x4d, 0x82, 0x14, 0x94, 0x84, 0x60, 0x37, 0xe2,
	0x12, 0x6d, 0xe2, 0x6d, 0xb2, 0xca, 0xcf, 0xba, 0xbb, 0x6b, 0xa0, 0x6f, 0xc2, 0x7b, 0xf0, 0x12,
	0x5c, 0xf2, 0x08, 0x28, 0xbc, 0x05, 0x57, 0x28, 0x5e, 0xb7, 0x08, 0x14, 0xa4, 0xde, 0x58, 0x67,
	0x8e, 0xcf, 0x9c, 0x39, 0x9a, 0x1d, 0xb8, 0xf9, 0x51, 0xc8, 0x39, 0x93, 0x8d, 0x84, 0x5e, 0x2c,
	0x04, 0x8d, 0x83, 0x44, 0x0a, 0x2d, 0xd0, 0x31, 0xac, 0xf7, 0x70, 0xca, 0xf5, 0x2c, 0x1d, 0x07,
	0x13, 0xb1, 0x6c, 0xc4, 0x53, 0x49, 0x93, 0xd9, 0x63, 0x2e, 0x72, 0xd4, 0xd0, 0x54, 0xcd, 0xb3,
	0x8f, 0x69, 0xf2, 0xef, 0x42, 0x71, 0x68, 0x5c, 0x10, 0xc1, 0x6e, 0x53, 0x4d, 0x0f, 0x49, 0x8d,
	0xd4, 0xf7, 0x42, 0x3b, 0xa6, 0x9a, 0xfa, 0x5f, 0x08, 0x54, 0x4e, 0xe8, 0x64, 0x9e, 0x26, 0x97,
	0xaa, 0x03, 0x70, 0x24, 0x3b, 0x7f, 0xcf, 0xe3, 0x4c, 0x67, 0x87, 0xff, 0x49, 0x76, 0xde, 0x8d,
	0xf1, 0x36, 0xec, 0x4e, 0xa5, 0x48, 0x93, 0xcd, 0x8f, 0x9d, 0x1a, 0xa9, 0x57, 0xc2, 0x62, 0x56,
	0x77, 0x63, 0x7c, 0x06, 0x8e, 0xd2, 0x54, 0xa7, 0xea, 0xb0, 0x50, 0x23, 0xf5, 0xff, 0x8f, 0xee,
	0x04, 0x26, 0x68, 0xf0, 0x87, 0x71, 0x10, 0x65, 0x9a, 0x30, 0xd7, 0xfa, 0x2f, 0xc1, 0x31, 0x0c,
	0xee, 0x82, 0x3d, 0x78, 0x33, 0xe8, 0xb8, 0x16, 0x96, 0xa1, 0x18, 0x8d, 0x5a, 0xad, 0x4e, 0x14,
	0xb9, 0x04, 0x2b, 0x50, 0x6a, 0x8f, 0x86, 0xbd, 0x6e, 0xab, 0x79, 0xda, 0x71, 0x77, 0x10, 0xc0,
	0x79, 0xd5, 0xec, 0xf6, 0x3a, 0x6d, 0xb7, 0x70, 0xf4, 0xb3, 0x00, 0xce, 0xbb, 0x6c, 0x06, 0x3e,
	0x00, 0xbb, 0x33, 0x99, 0x09, 0xdc, 0xbf, 0x1c, 0x9a, 0x8f, 0xf3, 0xfe, 0x26, 0x7c, 0x0b, 0xef,
	0x01, 0x34, 0x95, 0xe2, 0xd3, 0xd5, 0x88, 0xc7, 0x0a, 0x4b, 0x41, 0xb6, 0xa6, 0x41, 0xba, 0xf4,
	0xc0, 0xc0, 0x1e, 0x57, 0xda, 0xb7, 0xf0, 0x11, 0x38, 0xfd, 0x54, 0x53, 0xcd, 0x70, 0xdf, 0xf0,
	0x59, 0xc5, 0xc5, 0x4a, 0x6d, 0x33, 0xad, 0x43, 0x29, 0x62, 0xf2, 0x03, 0x3b, 0xa5, 0x6a, 0x8e,
	0x65, 0xd3, 0xf0, 0x36, 0x65, 0xf2, 0xc2, 0xdb, 0x33, 0x45, 0xc8, 0x54, 0xba, 0xd8, 0xf8, 0x3e,
	0x87, 0xca, 0x50, 0xb2, 0x98, 0x4f, 0xa8, 0x66, 0x9b, 0x87, 0xb8, 0x4e, 0xe6, 0x27, 0x04, 0xef,
	0x83, 0x1d, 0x09, 0xa9, 0x31, 0x0f, 0xb9, 0xc1, 0x9e, 0xfb, 0x1b, 0x5f, 0xd9, 0x3f, 0x85, 0x72,
	0x48, 0xcf, 0x74, 0x9f, 0x29, 0x45, 0xa7, 0xec, 0x5a, 0x0b, 0x39, 0x86, 0xf2, 0x6b, 0xc1, 0x57,
	0xad, 0x45, 0xaa, 0x34, 0x93, 0x78, 0x23, 0x0f, 0x4c, 0xcf, 0x74, 0x4b, 0xac, 0x34, 0xfb, 0xa4,
	0xb7, 0x35, 0xb5, 0xc1, 0x1d, 0x25, 0x31, 0xd5, 0xac, 0xcf, 0x96, 0x63, 0x26, 0xd5, 0x8c, 0x27,
	0x78, 0x2b, 0x5f, 0xd4, 0x15, 0x63, 0x14, 0xde, 0x3f, 0x78, 0xdf, 0xc2, 0x17, 0xe0, 0x98, 0xf3,
	0xc0, 0x83, 0xad, 0xe7, 0xe2, 0x6d, 0xa7, 0x7d, 0xeb, 0xc4, 0xfd, 0xba, 0xae, 0x92, 0x6f, 0xeb,
	0x2a, 0xf9, 0xbe, 0xae, 0x92, 0xcf, 0x3f, 0xaa, 0xd6, 0xd8, 0xc9, 0x4e, 0xfd, 0xf8, 0x57, 0x00,
	0x00, 0x00, 0xff, 0xff, 0xea, 0xd8, 0xbe, 0xa9, 0x37, 0x03, 0x00, 0x00,
}
