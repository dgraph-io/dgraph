//go:build oss
// +build oss

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

package worker

import (
	"math"

	"github.com/dgraph-io/dgraph/protos/pb"
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
