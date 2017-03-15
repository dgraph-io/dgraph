package task

import (
	"encoding/binary"

	"github.com/dgraph-io/dgraph/protos/taskp"
)

var (
	TrueVal  = FromBool(true)
	FalseVal = FromBool(false)
)

func FromInt(val int) *taskp.Value {
	bs := make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, uint32(val))
	return &taskp.Value{Val: []byte(bs), ValType: int32(2)}
}

func ToInt(val *taskp.Value) int32 {
	result := binary.LittleEndian.Uint32(val.Val)
	return int32(result)
}

func FromBool(val bool) *taskp.Value {
	if val == true {
		return FromInt(1)
	}
	return FromInt(0)
}

func ToBool(val *taskp.Value) bool {
	result := ToInt(val)
	if result != 0 {
		return true
	}
	return false
}
