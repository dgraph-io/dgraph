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

import (
	"github.com/dgraph-io/dgraph/query/pb"
	"github.com/dgraph-io/dgraph/x"
)

var glog = x.Log("client")

type Entity struct {
	Attribute string
	values    map[string][]byte
	gr        *pb.GraphResponse // Reference to iteratively build the entity
	uid       uint64
	children  []*Entity
}

// This function performs a binary search on the uids slice and returns the
// index at which it finds the uid, else returns -1
func indexOf(uid uint64, uids []uint64) int {
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

// This method is used to initialize the root entity
func NewEntity(root *pb.GraphResponse) *Entity {
	e := new(Entity)
	e.Attribute = root.Attribute
	e.uid = root.Result.Uidmatrix[0].Uids[0]
	e.gr = root
	return e
}

// This method populates and returns the list of properties for an entity.
func (e *Entity) Properties() []string {
	// This means properties have not been populated yet or the entity doesn't
	// have any properties.
	if len(e.values) == 0 {
		m := make(map[string][]byte)
		if e.gr == nil {
			glog.Fatalf("Nil graphResponse for entity with Attribute: %v",
				e.Attribute)
		}
		for _, grChild := range e.gr.Children {
			idx := indexOf(e.uid, grChild.Query.Uids)
			if idx == -1 {
				glog.Fatalf("Expected uid: %v to exist for Entity: %v in query uids"+
					"for child: %v", e.uid, e.Attribute, grChild.Attribute)
			}
			// This means its a leaf node and can be added as a property for enity.
			if len(grChild.Children) == 0 {
				m[grChild.Attribute] = grChild.Result.Values[idx]
			}
		}
		e.values = m
	}

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
	e.Properties()
	_, ok := e.values[property]
	return ok
}

// This method returns the value corresponding to a property if one exists.
func (e *Entity) Value(property string) []byte {
	e.Properties()
	val, _ := e.values[property]
	return val
}

// This method creates the children for an entity and returns them.
func (e *Entity) Children() []*Entity {
	// If length of children is zero, that means children have not been created
	// yet or no Child nodes exist
	if len(e.children) == 0 {
		if e.gr == nil {
			glog.Fatalf("Nil graphResponse for entity with Attribute: %v",
				e.Attribute)
		}
		for _, grChild := range e.gr.Children {
			// Index of the uid of the parent node in the query uids of the child.
			// This is used to get the appropriate uidList for the child
			idx := indexOf(e.uid, grChild.Query.Uids)
			if idx == -1 {
				glog.Fatalf("Expected uid: %v to exist for Entity: %v in query uids"+
					"for child: %v", e.uid, e.Attribute, grChild.Attribute)
			}
			if len(grChild.Children) != 0 {
				// Creating a child for each entry in the uidList
				for _, uid := range grChild.Result.Uidmatrix[idx].Uids {
					cEntity := new(Entity)
					cEntity.gr = grChild
					cEntity.Attribute = grChild.Attribute
					cEntity.uid = uid
					e.children = append(e.children, cEntity)
				}
			}
		}
	}
	return e.children
}

// This method returns the number of children for an entity.
func (e *Entity) NumChildren() int {
	return len(e.Children())
}
