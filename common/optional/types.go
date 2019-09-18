package optional

import (
	"fmt"

	common "github.com/ChainSafe/gossamer/common"
)

type Uint32 struct {
	exists bool
	value  uint32
}

func NewUint32(exists bool, value uint32) *Uint32 {
	return &Uint32{
		exists: exists,
		value:  value,
	}
}

func (x *Uint32) Exists() bool {
	return x.exists
}

func (x *Uint32) Value() uint32 {
	return x.value
}

func (x *Uint32) String() string {
	return fmt.Sprintf("%d", x.value)
}

func (x *Uint32) Set(exists bool, value uint32) {
	x.exists = exists
	x.value = value
}

type Hash struct {
	exists bool
	value  common.Hash
}

func NewHash(exists bool, value common.Hash) *Hash {
	return &Hash{
		exists: exists,
		value:  value,
	}
}

func (x *Hash) Exists() bool {
	return x.exists
}

func (x *Hash) Value() common.Hash {
	return x.value
}

func (x *Hash) String() string {
	return fmt.Sprintf("%x", x.value)
}

func (x *Hash) Set(exists bool, value common.Hash) {
	x.exists = exists
	x.value = value
}
