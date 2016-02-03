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
	"container/heap"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/x"
	"github.com/google/flatbuffers/go"
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

var glog = x.Log("query")

type Latency struct {
	Start      time.Time     `json:"-"`
	Parsing    time.Duration `json:"query_parsing"`
	Processing time.Duration `json:"processing"`
	Json       time.Duration `json:"json_conversion"`
}

func (l *Latency) ToMap() map[string]string {
	m := make(map[string]string)
	j := time.Since(l.Start) - l.Processing - l.Parsing
	m["parsing"] = l.Parsing.String()
	m["processing"] = l.Processing.String()
	m["json"] = j.String()
	m["total"] = time.Since(l.Start).String()
	return m
}

// SubGraph is the way to represent data internally. It contains both the
// query and the response. Once generated, this can then be encoded to other
// client convenient formats, like GraphQL / JSON.
type SubGraph struct {
	Attr     string
	Children []*SubGraph

	query  []byte
	result []byte
}

func mergeInterfaces(i1 interface{}, i2 interface{}) interface{} {
	switch i1.(type) {
	case map[string]interface{}:
		glog.Debug("Got map[string] interface")
		m1 := i1.(map[string]interface{})
		if m2, ok := i2.(map[string]interface{}); ok {
			for k1, v1 := range m1 {
				m2[k1] = v1
			}
			return m2
		}
		break
	}
	glog.Debugf("Got type: %v %v", reflect.TypeOf(i1), reflect.TypeOf(i2))
	glog.Debugf("Got values: %v %v", i1, i2)

	return []interface{}{i1, i2}
}

func postTraverse(g *SubGraph) (result map[uint64]interface{}, rerr error) {
	if len(g.query) == 0 {
		return result, nil
	}

	result = make(map[uint64]interface{})
	// Get results from all children first.
	cResult := make(map[uint64]interface{})

	for _, child := range g.Children {
		m, err := postTraverse(child)
		if err != nil {
			x.Err(glog, err).Error("Error while traversal")
			return result, err
		}
		// Merge results from all children, one by one.
		for k, v := range m {
			if val, present := cResult[k]; !present {
				cResult[k] = v
			} else {
				cResult[k] = mergeInterfaces(val, v)
			}
		}
	}

	// Now read the query and results at current node.
	uo := flatbuffers.GetUOffsetT(g.query)
	q := new(task.Query)
	q.Init(g.query, uo)

	ro := flatbuffers.GetUOffsetT(g.result)
	r := new(task.Result)
	r.Init(g.result, ro)

	if q.UidsLength() != r.UidmatrixLength() {
		glog.Fatal("Result uidmatrixlength: %v. Query uidslength: %v",
			r.UidmatrixLength(), q.UidsLength())
	}
	if q.UidsLength() != r.ValuesLength() {
		glog.Fatalf("Result valuelength: %v. Query uidslength: %v",
			r.ValuesLength(), q.UidsLength())
	}

	var ul task.UidList
	for i := 0; i < r.UidmatrixLength(); i++ {
		if ok := r.Uidmatrix(&ul, i); !ok {
			return result, fmt.Errorf("While parsing UidList")
		}
		l := make([]interface{}, ul.UidsLength())
		for j := 0; j < ul.UidsLength(); j++ {
			uid := ul.Uids(j)
			m := make(map[string]interface{})
			m["_uid_"] = fmt.Sprintf("%#x", uid)
			if ival, present := cResult[uid]; !present {
				l[j] = m
			} else {
				l[j] = mergeInterfaces(m, ival)
			}
		}
		if len(l) > 0 {
			m := make(map[string]interface{})
			m[g.Attr] = l
			result[q.Uids(i)] = m
		}
	}

	var tv task.Value
	for i := 0; i < r.ValuesLength(); i++ {
		if ok := r.Values(&tv, i); !ok {
			return result, fmt.Errorf("While parsing value")
		}
		var ival interface{}
		if err := posting.ParseValue(&ival, tv.ValBytes()); err != nil {
			return result, err
		}
		if ival == nil {
			continue
		}

		if pval, present := result[q.Uids(i)]; present {
			glog.WithField("prev", pval).
				WithField("_uid_", q.Uids(i)).
				WithField("new", ival).
				Fatal("Previous value detected.")
		}
		m := make(map[string]interface{})
		m["_uid_"] = fmt.Sprintf("%#x", q.Uids(i))
		glog.WithFields(logrus.Fields{
			"_uid_": q.Uids(i),
			"val":   ival,
		}).Debug("Got value")
		m[g.Attr] = ival
		result[q.Uids(i)] = m
	}
	return result, nil
}

func (g *SubGraph) ToJson(l *Latency) (js []byte, rerr error) {
	r, err := postTraverse(g)
	if err != nil {
		x.Err(glog, err).Error("While doing traversal")
		return js, err
	}
	l.Json = time.Since(l.Start) - l.Parsing - l.Processing
	if len(r) == 1 {
		for _, ival := range r {
			m := ival.(map[string]interface{})
			m["server_latency"] = l.ToMap()
			return json.Marshal(m)
		}
	} else {
		glog.Fatal("We don't currently support more than 1 uid at root.")
	}

	glog.Fatal("Shouldn't reach here.")
	return json.Marshal(r)
}

func NewGraph(euid uint64, exid string) (*SubGraph, error) {
	// This would set the Result field in SubGraph,
	// and populate the children for attributes.
	if len(exid) > 0 {
		u, err := uid.GetOrAssign(exid, 0, 1) // instanceIdx = 0, numInstances = 1 by default
		if err != nil {
			x.Err(glog, err).WithField("xid", exid).Error(
				"While GetOrAssign uid from external id")
			return nil, err
		}
		glog.WithField("xid", exid).WithField("_uid_", u).Debug("GetOrAssign")
		euid = u
	}

	if euid == 0 {
		err := fmt.Errorf("Query internal id is zero")
		x.Err(glog, err).Error("Invalid query")
		return nil, err
	}

	// Encode uid into result flatbuffer.
	b := flatbuffers.NewBuilder(0)
	omatrix := x.UidlistOffset(b, []uint64{euid})

	// Also need to add nil value to keep this consistent.
	var voffset flatbuffers.UOffsetT
	{
		bvo := b.CreateByteVector(x.Nilbyte)
		task.ValueStart(b)
		task.ValueAddVal(b, bvo)
		voffset = task.ValueEnd(b)
	}

	task.ResultStartUidmatrixVector(b, 1)
	b.PrependUOffsetT(omatrix)
	mend := b.EndVector(1)

	task.ResultStartValuesVector(b, 1)
	b.PrependUOffsetT(voffset)
	vend := b.EndVector(1)

	task.ResultStart(b)
	task.ResultAddUidmatrix(b, mend)
	task.ResultAddValues(b, vend)
	rend := task.ResultEnd(b)
	b.Finish(rend)

	sg := new(SubGraph)
	sg.Attr = "_root_"
	sg.result = b.Bytes[b.Head():]
	// Also add query for consistency and to allow for ToJson() later.
	sg.query = createTaskQuery(sg.Attr, []uint64{euid})
	return sg, nil
}

// createTaskQuery generates the query buffer.
func createTaskQuery(attr string, sorted []uint64) []byte {
	b := flatbuffers.NewBuilder(0)
	ao := b.CreateString(attr)

	task.QueryStartUidsVector(b, len(sorted))
	for i := len(sorted) - 1; i >= 0; i-- {
		b.PrependUint64(sorted[i])
	}
	vend := b.EndVector(len(sorted))

	task.QueryStart(b)
	task.QueryAddAttr(b, ao)
	task.QueryAddUids(b, vend)
	qend := task.QueryEnd(b)
	b.Finish(qend)
	return b.Bytes[b.Head():]
}

type ListChannel struct {
	TList *task.UidList
	Idx   int
}

func sortedUniqueUids(r *task.Result) (sorted []uint64, rerr error) {
	// Let's serialize the matrix of uids in result to a
	// sorted unique list of uids.
	h := &x.Uint64Heap{}
	heap.Init(h)

	channels := make([]*ListChannel, r.UidmatrixLength())
	for i := 0; i < r.UidmatrixLength(); i++ {
		tlist := new(task.UidList)
		if ok := r.Uidmatrix(tlist, i); !ok {
			return sorted, fmt.Errorf("While parsing Uidmatrix")
		}
		if tlist.UidsLength() > 0 {
			e := x.Elem{
				Uid: tlist.Uids(0),
				Idx: i,
			}
			heap.Push(h, e)
		}
		channels[i] = &ListChannel{TList: tlist, Idx: 1}
	}

	// The resulting list of uids will be stored here.
	sorted = make([]uint64, 100)
	sorted = sorted[:0]

	var last uint64
	last = 0
	// Itearate over the heap.
	for h.Len() > 0 {
		me := (*h)[0] // Peek at the top element in heap.
		if me.Uid != last {
			sorted = append(sorted, me.Uid) // Add if unique.
			last = me.Uid
		}
		lc := channels[me.Idx]
		if lc.Idx >= lc.TList.UidsLength() {
			heap.Pop(h)

		} else {
			uid := lc.TList.Uids(lc.Idx)
			lc.Idx += 1

			me.Uid = uid
			(*h)[0] = me
			heap.Fix(h, 0) // Faster than Pop() followed by Push().
		}
	}
	return sorted, nil
}

func ProcessGraph(sg *SubGraph, rch chan error) {
	var err error
	if len(sg.query) > 0 && sg.Attr != "_root_" {
		// This task execution would go over the wire in later versions.
		sg.result, err = posting.ProcessTask(sg.query)
		if err != nil {
			x.Err(glog, err).Error("While processing task.")
			rch <- err
			return
		}
	}

	uo := flatbuffers.GetUOffsetT(sg.result)
	r := new(task.Result)
	r.Init(sg.result, uo)

	sorted, err := sortedUniqueUids(r)
	if err != nil {
		x.Err(glog, err).Error("While processing task.")
		rch <- err
		return
	}

	if len(sorted) == 0 {
		// Looks like we're done here.
		if len(sg.Children) > 0 {
			glog.Debugf("Have some children but no results. Life got cut short early."+
				"Current attribute: %q", sg.Attr)
		} else {
			glog.Debugf("No more things to process for Attr: %v", sg.Attr)
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
		child.query = createTaskQuery(child.Attr, sorted)
		go ProcessGraph(child, childchan)
	}

	// Now get all the results back.
	for i := 0; i < len(sg.Children); i++ {
		err = <-childchan
		glog.WithFields(logrus.Fields{
			"num_children": len(sg.Children),
			"index":        i,
			"attr":         sg.Children[i].Attr,
			"err":          err,
		}).Debug("Reply from child")
		if err != nil {
			x.Err(glog, err).Error("While processing child task.")
			rch <- err
			return
		}
	}
	rch <- nil
}
