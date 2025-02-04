/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package index

// TypeID represents the type of the data.
type TypeID Posting_ValType
type Posting_ValType int32

type KeyValue struct {
	// Entity is the uid of the key to be built
	Entity uint64 `protobuf:"fixed64,1,opt,name=entity,proto3" json:"entity,omitempty"`
	// Attr is the attribute of the key to be built
	Attr string `protobuf:"bytes,2,opt,name=attr,proto3" json:"attr,omitempty"`
	// Value is the value corresponding to the key built from entity & attr
	Value []byte `protobuf:"bytes,3,opt,name=value,proto3" json:"value,omitempty"`
}
