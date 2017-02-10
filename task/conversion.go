
package task

import (
	"encoding/binary"
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
