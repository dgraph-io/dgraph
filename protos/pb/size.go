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

package pb

// DeepSize computes the memory consumption of pb.PostingList
func (m *PostingList) DeepSize() int {
	var size int

	if m == nil {
		return size
	}

	return size
}

// DeepSize computes the memory consumption of pb.UidPack
func (m *Posting) DeepSize() int {
	var size int

	if m == nil {
		return size
	}

	return size
}

// DeepSize computes the memory consumption of pb.UidPack
func (m *UidPack) DeepSize() int {
	var size int

	if m == nil {
		return size
	}

	// size of struct UidPack (1 + 3 + 0 + 3 + 1)
	// UidPack.BlockSize consumes a full word
	size += 8 * 8

	// UidPack.Blocks, each pointer takes 1 word
	size += cap(m.Blocks) * 8

	// UidPack.XXX_unrecognized
	size += cap(m.XXX_unrecognized)

	return size
}

// DeepSize computes the memory consumption of pb.UidBlock
func (m *UidBlock) DeepSize() int {
	var size int

	if m == nil {
		return size
	}

	// size of struct UidBlock (1 + 3 + 1 + 0 + 3 + 1)
	// Due to alignment, both NumUids and XXX_sizecache consume 8 bytes instead of 4 bytes
	size += 9 * 8

	// UidBlock.Deltas
	size += cap(m.Deltas)

	// UidBlock.XXX_unrecognized
	size += cap(m.XXX_unrecognized)

	return size
}
