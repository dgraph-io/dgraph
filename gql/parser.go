/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package gql

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/x"
)

func Parse(input string) (sg *query.SubGraph, rerr error) {
	l := newLexer(input)
	sg = nil
	for item := range l.items {
		if item.typ == itemText {
			continue
		}
		if item.typ == itemOpType {
			if item.val == "mutation" {
				return nil, errors.New("Mutations not supported")
			}
		}
		if item.typ == itemLeftCurl {
			if sg == nil {
				sg, rerr = getRoot(l)
				if rerr != nil {
					x.Err(glog, rerr).Error("While retrieving subgraph root")
					return nil, rerr
				}
			} else {
				godeep(l, sg)
			}
		}
	}
	return sg, nil
}

func getRoot(l *lexer) (sg *query.SubGraph, rerr error) {
	item := <-l.items
	if item.typ != itemName {
		return nil, fmt.Errorf("Expected some name. Got: %v", item)
	}
	// ignore itemName for now.
	item = <-l.items
	if item.typ != itemLeftRound {
		return nil, fmt.Errorf("Expected variable start. Got: %v", item)
	}

	var uid uint64
	var xid string
	for {
		var key, val string
		// Get key or close bracket
		item = <-l.items
		if item.typ == itemArgName {
			key = item.val
		} else if item.typ == itemRightRound {
			break
		} else {
			return nil, fmt.Errorf("Expecting argument name. Got: %v", item)
		}

		// Get corresponding value.
		item = <-l.items
		if item.typ == itemArgVal {
			val = item.val
		} else {
			return nil, fmt.Errorf("Expecting argument val. Got: %v", item)
		}

		if key == "uid" {
			uid, rerr = strconv.ParseUint(val, 0, 64)
			if rerr != nil {
				return nil, rerr
			}
		} else if key == "xid" {
			xid = val
		} else {
			return nil, fmt.Errorf("Expecting uid or xid. Got: %v", item)
		}
	}
	if item.typ != itemRightRound {
		return nil, fmt.Errorf("Unexpected token. Got: %v", item)
	}
	return query.NewGraph(uid, xid)
}

func godeep(l *lexer, sg *query.SubGraph) {
	curp := sg // stores current pointer.
	for {
		switch item := <-l.items; {
		case item.typ == itemName:
			child := new(query.SubGraph)
			child.Attr = item.val
			sg.Children = append(sg.Children, child)
			curp = child
		case item.typ == itemLeftCurl:
			godeep(l, curp) // recursive iteration
		case item.typ == itemRightCurl:
			return
		case item.typ == itemLeftRound:
			// absorb all these, we don't care right now.
			for {
				item = <-l.items
				if item.typ == itemRightRound || item.typ == itemEOF {
					break
				}
			}
		default:
			// continue
		}
	}

}
