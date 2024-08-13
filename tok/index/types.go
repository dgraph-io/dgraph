/*
 * Copyright 2016-2024 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Co-authored by: jairad26@gmail.com, sunil@hypermode.com, bill@hypdermode.com
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
