/*
 * Copyright 2016 DGraph Labs, Inc.
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

package client

import "github.com/dgraph-io/dgraph/query/pb"

type Entity struct {
	Attribute string
	values    map[string][]byte
	gr        *pb.GraphResponse // Reference to iteratively build the entity
	uid       uint64
	children  []*Entity
}

// This function performs a binary search on the uids slice and returns the
// index at which it finds the uid, else returns -1
func search(uid uint64, uids []uint64) int {
	low, mid, high := 0, 0, len(uids)-1
	for low <= high {
		mid = (low + high) / 2
		if uids[mid] == uid {
			return mid
		} else if uids[mid] > uid {
			high = mid - 1
		} else {
			low = mid + 1
		}
	}
	return -1
}

// This method populates the values and children for an entity
func (e *Entity) populate() {
	m := make(map[string][]byte)
	for _, grChild := range e.gr.Children {
		// Index of the uid of the parent node in the query uids of the child. This
		// is used to get the appropriate value or uidlist for the child.
		// TODO(pawan) - Log error if i == -1
		i := search(e.uid, grChild.Query.Uids)
		// This means its a leaf node
		if len(grChild.Children) == 0 {
			m[grChild.Attribute] = grChild.Result.Values[i]
		} else {
			// An entity would have as many children as the length of UidList for its
			// child node.
			for _, uid := range grChild.Result.Uidmatrix[i].Uids {
				cEntity := new(Entity)
				cEntity.gr = grChild
				cEntity.Attribute = grChild.Attribute
				cEntity.uid = uid
				e.children = append(e.children, cEntity)
			}
		}
	}
	e.values = m
}

// This method is used to initialize the root entity
func NewEntity(root *pb.GraphResponse) *Entity {
	e := new(Entity)
	e.Attribute = root.Attribute
	e.uid = root.Result.Uidmatrix[0].Uids[0]
	e.gr = root
	e.populate()
	return e
}

// This method returns the list of properties for an entity.
func (e *Entity) Properties() []string {
	properties := make([]string, len(e.values))
	i := 0
	for k := range e.values {
		properties[i] = k
		i++
	}
	return properties
}

// This method returns whether a property exists for an entity or not.
func (e *Entity) HasValue(property string) bool {
	_, ok := e.values[property]
	return ok
}

// This method returns the value corresponding to a property if one exists.
func (e *Entity) Value(property string) []byte {
	val, _ := e.values[property]
	return val
}

// This method populates the children for an entity and returns them.
func (e *Entity) Children() []*Entity {
	for _, child := range e.children {
		// This makes sure that children are populated only once.
		// TODO(pawan) - Discuss with Manish if it makes sense to have a flag for
		// this.
		if len(child.children) == 0 && child.values == nil {
			child.populate()
		}
	}
	return e.children
}

// This method returns the number of children for an entity.
func (e *Entity) NumChildren() int {
	return len(e.children)
}
