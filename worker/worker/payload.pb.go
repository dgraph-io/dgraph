// Code generated by protoc-gen-go.
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

// discarding unused import task "github.com/dgraph-io/dgraph/task"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal

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

type Payload struct {
	Data []byte `protobuf:"bytes,1,opt,proto3" json:"Data,omitempty"`
}

func (m *Payload) Reset()         { *m = Payload{} }
func (m *Payload) String() string { return proto.CompactTextString(m) }
func (*Payload) ProtoMessage()    {}

// BackupPayload is used both as a request and a response.
// When used in request, groups represents the list of groups that need to be backed up.
// When used in response, groups represent the list of groups that were backed up.
type BackupPayload struct {
	ReqId   uint64               `protobuf:"varint,1,opt,name=req_id" json:"req_id,omitempty"`
	GroupId uint32               `protobuf:"varint,2,opt,name=group_id" json:"group_id,omitempty"`
	Status  BackupPayload_Status `protobuf:"varint,3,opt,name=status,enum=worker.BackupPayload_Status" json:"status,omitempty"`
}

func (m *BackupPayload) Reset()         { *m = BackupPayload{} }
func (m *BackupPayload) String() string { return proto.CompactTextString(m) }
func (*BackupPayload) ProtoMessage()    {}

func init() {
	proto.RegisterEnum("worker.BackupPayload_Status", BackupPayload_Status_name, BackupPayload_Status_value)
}
