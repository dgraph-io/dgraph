/*
 * Copyright 2016-2019 Dgraph Labs, Inc. and Contributors
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
	"github.com/dgraph-io/dgraph/protos/pb"
)

// IsListFull check whether the posting List is full based on the limit
// by default it return false, if the limit is set to zero.
func IsListFull(l *pb.List, limit int32) bool {
	return limit != 0 && len(l.Uids) == int(limit)
}
