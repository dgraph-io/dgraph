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

	// 1+3+1+3+0+3+1
	size += 12 * 8

	// PostingList.Pack
	size += m.Pack.DeepSize()

	// PostingList.Postings
	size += cap(m.Postings) * 8
	for _, p := range m.Postings {
		size += p.DeepSize()
	}

	// PostingList.Splits
	size += cap(m.Splits) * 8

	// PostingList.XXX_unrecognized
	size += cap(m.XXX_unrecognized)

	return size
}

// DeepSize computes the memory consumption of pb.UidPack
func (m *Posting) DeepSize() int {
	var size int

	if m == nil {
		return size
	}

	// 1 + 3 + 1 + 1 + 3 + 2 + 3 + 1 + 1 + 1 + 0 + 3 + 1
	size += 22 * 8

	// Posting.Value
	size += cap(m.Value)

	// Posting.LangTag
	size += cap(m.LangTag)

	// Posting.Label, strings are immutable, hence cap = len
	size += len(m.Label)

	// posting.Facets
	// struct size for api.Facets: 2 + 3 + 1 + 3 + 2 + 0 + 3 + 1
	size += cap(m.Facets) * 8
	size += 15
	for _, f := range m.Facets {
		// api.Facet.Key
		size += len(f.Key)

		// api.Facet.Value
		size += cap(f.Value)

		// api.Facet.Tokens, space taken by string slice
		size += cap(f.Tokens) * 16

		// space taken by each string element in m.Facets.Tokens
		for _, s := range f.Tokens {
			size += len(s)
		}

		// api.Facet.Alias
		size += len(f.Alias)

		// api.Facet.XXX_unrecognized
		size += cap(f.XXX_unrecognized)
	}

	// Posting.XXX_unrecognized
	size += cap(m.XXX_unrecognized)

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
	// to round the word making it 10
	size += 10 * 8

	// UidBlock.Deltas
	size += cap(m.Deltas)

	// UidBlock.XXX_unrecognized
	size += cap(m.XXX_unrecognized)

	return size
}
