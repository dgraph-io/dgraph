/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package task

import (
	"encoding/binary"

	"github.com/dgraph-io/dgraph/protos/pb"
)

var (
	TrueVal  = FromBool(true)
	FalseVal = FromBool(false)
)

func FromInt(val int) *pb.TaskValue {
	bs := make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, uint32(val))
	return &pb.TaskValue{Val: []byte(bs), ValType: pb.Posting_INT}
}

func ToInt(val *pb.TaskValue) int32 {
	result := binary.LittleEndian.Uint32(val.Val)
	return int32(result)
}

func FromBool(val bool) *pb.TaskValue {
	if val == true {
		return FromInt(1)
	}
	return FromInt(0)
}

func ToBool(val *pb.TaskValue) bool {
	if len(val.Val) == 0 {
		return false
	}
	result := ToInt(val)
	if result != 0 {
		return true
	}
	return false
}
