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

package query

import (
	"fmt"

	"github.com/google/flatbuffers/go"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/x"
)

/*
 * QUERY:
 * Let's take this query from GraphQL as example:
 * {
 *   me {
 *     id
 *     firstName
 *     lastName
 *     birthday {
 *       month
 *       day
 *     }
 *     friends {
 *       name
 *     }
 *   }
 * }
 *
 * REPRESENTATION:
 * This would be represented in SubGraph format internally, as such:
 * SubGraph [result uid = me]
 *    |
 *  Children
 *    |
 *    --> SubGraph [Attr = "xid"]
 *    --> SubGraph [Attr = "firstName"]
 *    --> SubGraph [Attr = "lastName"]
 *    --> SubGraph [Attr = "birthday"]
 *           |
 *         Children
 *           |
 *           --> SubGraph [Attr = "month"]
 *           --> SubGraph [Attr = "day"]
 *    --> SubGraph [Attr = "friends"]
 *           |
 *         Children
 *           |
 *           --> SubGraph [Attr = "name"]
 *
 * ALGORITHM:
 * This is a rough and simple algorithm of how to process this SubGraph query
 * and populate the results:
 *
 * For a given entity, a new SubGraph can be started off with NewGraph(id).
 * Given a SubGraph, is the Query field empty? [Step a]
 *   - If no, run (or send it to server serving the attribute) query
 *     and populate result.
 * Iterate over children and copy Result Uids to child Query Uids.
 *     Set Attr. Then for each child, use goroutine to run Step:a.
 * Wait for goroutines to finish.
 * Return errors, if any.
 */

var log = x.Log("query")

// SubGraph is the way to represent data internally. It contains both the
// query and the response. Once generated, this can then be encoded to other
// client convenient formats, like GraphQL / JSON.
type SubGraph struct {
	Attr     string
	Children []*SubGraph

	query  []byte
	result []byte
}

func NewGraph(euid uint64, exid string) (*SubGraph, error) {
	// This would set the Result field in SubGraph,
	// and populate the children for attributes.
	if len(exid) > 0 {
		u, err := uid.GetOrAssign(exid)
		if err != nil {
			x.Err(log, err).WithField("xid", exid).Error(
				"While GetOrAssign uid from external id")
			return nil, err
		}
		log.WithField("xid", exid).WithField("uid", u).Debug("GetOrAssign")
		euid = u
	}

	if euid == 0 {
		err := fmt.Errorf("Query internal id is zero")
		x.Err(log, err).Error("Invalid query")
		return nil, err
	}

	// Encode uid into result flatbuffer.
	b := flatbuffers.NewBuilder(0)
	task.ResultStartUidsVector(b, 1)
	b.PrependUint64(euid)
	vend := b.EndVector(1)
	task.ResultStart(b)
	task.ResultAddUids(b, vend)
	rend := task.ResultEnd(b)
	b.Finish(rend)

	sg := new(SubGraph)
	sg.result = b.Bytes[b.Head():]
	return sg, nil
}

func createTaskQuery(attr string, r *task.Result) []byte {
	b := flatbuffers.NewBuilder(0)
	ao := b.CreateString(attr)

	task.QueryStartUidsVector(b, r.UidsLength())
	for i := r.UidsLength() - 1; i >= 0; i-- {
		uid := r.Uids(i)
		b.PrependUint64(uid)
	}
	vend := b.EndVector(r.UidsLength())

	task.QueryStart(b)
	task.QueryAddAttr(b, ao)
	task.QueryAddUids(b, vend)
	qend := task.QueryEnd(b)
	b.Finish(qend)
	return b.Bytes[b.Head():]
}

func ProcessGraph(sg *SubGraph, rch chan error) {
	var err error
	if len(sg.query) > 0 {
		// This task execution would go over the wire in later versions.
		sg.result, err = posting.ProcessTask(sg.query)
		if err != nil {
			rch <- err
			return
		}
	}

	uo := flatbuffers.GetUOffsetT(sg.result)
	r := new(task.Result)
	r.Init(sg.result, uo)
	if r.UidsLength() == 0 {
		// Looks like we're done here.
		if len(sg.Children) > 0 {
			log.Debug("Have some children but no results. Life got cut short early.")
		}
		rch <- nil
		return
	}

	// Let's execute it in a tree fashion. Each SubGraph would break off
	// as many goroutines as it's children; which would then recursively
	// do the same thing.
	// Buffered channel to ensure no-blockage.
	childchan := make(chan error, len(sg.Children))
	for i := 0; i < len(sg.Children); i++ {
		child := sg.Children[i]
		child.query = createTaskQuery(child.Attr, r)
		go ProcessGraph(child, childchan)
	}

	// Now get all the results back.
	for i := 0; i < len(sg.Children); i++ {
		err = <-childchan
		if err != nil {
			rch <- err
			return
		}
	}
	rch <- nil
}
