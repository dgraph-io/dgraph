/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package y contains the code shared by the Go client and Dgraph server. It should have
// minimum dependencies in order to keep the client light.
package y

import (
	"errors"

	"github.com/dgraph-io/dgraph/protos"
)

var (
	ErrAborted = errors.New("Transaction has been aborted due to conflict")
)

func MergeLinReads(dst *protos.LinRead, src *protos.LinRead) {
	if src == nil || src.Ids == nil {
		return
	}
	if dst.Ids == nil {
		dst.Ids = make(map[uint32]uint64)
	}
	for gid, sid := range src.Ids {
		if did, has := dst.Ids[gid]; has && did >= sid {
			// do nothing.
		} else {
			dst.Ids[gid] = sid
		}
	}
}
