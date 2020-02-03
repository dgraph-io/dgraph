/*
 * Copyright 2017-2020 Dgraph Labs, Inc. and Contributors
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
package bulk

import "github.com/dgraph-io/dgraph/protos/pb"

type Iterator interface {
	Next()
	Current() *pb.MapEntry
}

type MergeIterator struct {
	left  node
	right node
	small *node
}

type node struct {
	iter     Iterator
	mapItr   *mapIterator
	mergeItr *MergeIterator
}

func (n *node) current() *pb.MapEntry {
	return n.iter.Current()
}

func (mi *MergeIterator) bigger() *node {
	if mi.small == &mi.left {
		return &mi.right
	}
	return &mi.left
}

func (mi *MergeIterator) swapSmall() {
	if mi.small == &mi.left {
		mi.small = &mi.right
		return
	}
	if mi.small == &mi.right {
		mi.small = &mi.left
		return
	}
}

func (mi *MergeIterator) Next() {
	mi.small.iter.Next()
	mi.fix()
}

func (mi *MergeIterator) fix() {
	if mi.bigger().current() == nil {
		return
	}
	if mi.small.current() == nil {
		mi.swapSmall()
		return
	}
	if !less(mi.small.current(), mi.bigger().current()) {
		mi.swapSmall()
	}
}

func (mi *MergeIterator) Current() *pb.MapEntry {
	return mi.small.current()
}

func NewMergeIterator(itrs []Iterator) Iterator {
	switch len(itrs) {
	case 0:
		return nil
	case 1:
		return itrs[0]
	case 2:
		mi := &MergeIterator{
			left: node{
				iter: itrs[0],
			},
			right: node{
				iter: itrs[1],
			},
		}
		mi.small = &mi.left
		mi.fix()
		return mi
	}
	mid := len(itrs) / 2
	return NewMergeIterator([]Iterator{NewMergeIterator(itrs[:mid]),
		NewMergeIterator(itrs[mid:])})
}
