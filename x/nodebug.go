//go:build !debug
// +build !debug

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"github.com/dgraph-io/badger/v4"
	bpb "github.com/dgraph-io/badger/v4/pb"
	"github.com/hypermodeinc/dgraph/v25/protos/pb"
)

// PrintRollup prints information about any rollup that happen
func PrintRollup(plist *pb.PostingList, parts map[uint64]*pb.PostingList, baseKey []byte, ts uint64) {
}

// PrintMutationEdge prints all edges that are being inserted into badger
func PrintMutationEdge(plist *pb.DirectedEdge, key ParsedKey, startTs uint64) {
}

// PrintOracleDelta prints all delta proposals that are commited
func PrintOracleDelta(delta *pb.OracleDelta) {
}

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
