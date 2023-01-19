//go:build !debug
// +build !debug

/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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
 */

package x

import (
	"github.com/dgraph-io/badger/v3"
	bpb "github.com/dgraph-io/badger/v3/pb"
	"github.com/dgraph-io/dgraph/protos/pb"
)

// VerifyPack works in debug mode. Check out the comment in debug_on.go
func VerifyPack(plist *pb.PostingList) {
}

// VerifySnapshot works in debug mode. Check out the comment in debug_on.go
func VerifySnapshot(pstore *badger.DB, readTs uint64) {
}

// VerifyPostingSplits works in debug mode. Check out the comment in debug_on.go
func VerifyPostingSplits(kvs []*bpb.KV, plist *pb.PostingList,
	parts map[uint64]*pb.PostingList, baseKey []byte) {
}
