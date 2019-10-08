/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package posting

// DeepSize computes the memory taken by a Posting List
func (l *List) DeepSize() int {
	var size int

	if l == nil {
		return size
	}

	// size of List struct
	// SafeMutex (4 words) + key (3 words, len,cap,pointer) + plist (1 word) +
	// mutationMap (1 word, map is just a pointer) + minTs & maxTs (2 words) +
	// 1 word padding
	size += 12

	// size inside key
	size += cap(l.key)

	// size inside plist
	if l.plist != nil {
		size += l.plist.DeepSize()
	}

	// map size computation

	// memory taken in PostingList in Map
	for _, v := range l.mutationMap {
		size += v.DeepSize()
	}

	return size
}
