
package task

import (
	"encoding/binary"
)

var (
	TrueVal  = FromBool(true)
	FalseVal = FromBool(false)
)

func FromInt(val int) *Value {
	bs := make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, uint32(val))
	return &Value{Val:[]byte(bs), ValType:int32(2)}
}

func ToInt(val *Value) int32 {
	result := binary.LittleEndian.Uint32(val.Val)
	return int32(result)
}

func FromBool(val bool) *Value {
	if val == true {
		return FromInt(1)
	}
	return FromInt(0)
}

func ToBool(val *Value) bool {
	result := ToInt(val)
	if result != 0 {
		return true
	}
	return false
}
