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
	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/query/pb"
	"github.com/dgraph-io/dgraph/x"
)

var glog = x.Log("client")

type Entity struct {
	Attribute string
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

// This method returns the list of properties for an entity.
func (e *Entity) Properties() []string {
	m := []string{}
	if e.gr == nil {
		glog.WithField("attribute", e.Attribute).Fatal("Nil graphResponse")
		return m
	}

	for _, predChild := range e.gr.Children {
		// Skipping non-leaf nodes.
		if len(predChild.Children) > 0 {
			continue
		}
		m = append(m, predChild.Attribute)
	}
	return m
}

// This method returns whether a property exists for an entity or not.
func (e *Entity) HasValue(property string) bool {
	for _, predChild := range e.gr.Children {
		if len(predChild.Children) > 0 {
			continue
		}
		if predChild.Attribute == property {
			return true
		}
	}
	return false
}

// This method returns the value corresponding to a property if one exists.
func (e *Entity) Value(property string) []byte {
	for _, predChild := range e.gr.Children {
		if len(predChild.Children) > 0 || predChild.Attribute != property {
			continue
		}
		idx := indexOf(e.uid, predChild.Query.Uids)
		if idx == -1 {
			glog.WithFields(logrus.Fields{
				"uid":            e.uid,
				"attribute":      e.Attribute,
				"childAttribute": predChild.Attribute,
			}).Fatal("Attribute with uid not found in child Query uids")
			return []byte{}
		}
		return predChild.Result.Values[idx]
	}
	return []byte{}
}

// This method creates the children for an entity and returns them.
func (e *Entity) Children() []*Entity {
	var children []*Entity
	// If length of children is zero, that means children have not been created
	// yet or no Child nodes exist
	if len(e.children) > 0 {
		return e.children
	}
	if e.gr == nil {
		glog.WithField("attribute", e.Attribute).Fatal("Nil graphResponse")
		return children
	}
	for _, predChild := range e.gr.Children {
		// Index of the uid of the parent node in the query uids of the child.
		// This is used to get the appropriate uidList for the child
		idx := indexOf(e.uid, predChild.Query.Uids)
		if idx == -1 {
			glog.WithFields(logrus.Fields{
				"uid":            e.uid,
				"attribute":      e.Attribute,
				"childAttribute": predChild.Attribute,
			}).Fatal("Attribute with uid not found in child Query uids")
			return children
		}
		if len(predChild.Children) != 0 {
			// Creating a child for each entry in the uidList
			for _, uid := range predChild.Result.Uidmatrix[idx].Uids {
				cEntity := new(Entity)
				cEntity.gr = predChild
				cEntity.Attribute = predChild.Attribute
				cEntity.uid = uid
				children = append(children, cEntity)
			}
		}
	}
	e.children = children
	return children
}

// This method returns the number of children for an entity.
func (e *Entity) NumChildren() int {
	return len(e.Children())
}
