/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package task

import (
	"encoding/binary"

	"github.com/dgraph-io/dgraph/protos/intern"
)

var (
	TrueVal  = FromBool(true)
	FalseVal = FromBool(false)
)

func FromInt(val int) *intern.TaskValue {
	bs := make([]byte, 8)
	binary.LittleEndian.PutUint64(bs, uint64(val))
	return &intern.TaskValue{Val: []byte(bs), ValType: intern.Posting_INT}
}

func ToInt(val *intern.TaskValue) int64 {
	result := binary.LittleEndian.Uint64(val.Val)
	return int64(result)
}

func FromBool(val bool) *intern.TaskValue {
	if val == true {
		return FromInt(1)
	}
	return FromInt(0)
}

func ToBool(val *intern.TaskValue) bool {
	if len(val.Val) == 0 {
		return false
	}
	result := ToInt(val)
	if result != 0 {
		return true
	}
	return false
}
