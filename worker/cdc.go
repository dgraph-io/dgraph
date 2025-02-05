//go:build oss
// +build oss

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"math"

	"github.com/hypermodeinc/dgraph/v24/protos/pb"
)

type CDC struct {
}

func newCDC() *CDC {
	return nil
}

func (cd *CDC) getTs() uint64 {
	return math.MaxUint64
}

func (cd *CDC) updateTs(ts uint64) {
	return
}

func (cdc *CDC) getSeenIndex() uint64 {
	return math.MaxUint64
}

func (cdc *CDC) updateCDCState(state *pb.CDCState) {
	return
}

func (cd *CDC) Close() {
	return
}

// todo: test cases old cluster restart, live loader, bulk loader, backup restore etc
func (cd *CDC) processCDCEvents() {
	return
}
