/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package query

import (
	"container/heap"
	"context"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
)

type pathInfo struct {
	uid   uint64
	attr  string
	facet *protos.Facets
}

type route struct {
	route []pathInfo
}

type Item struct {
	uid   uint64  // uid of the node.
	cost  float64 // cost of taking the path till this uid.
	hop   int     // number of hops taken to reach this node.
	index int
	path  route // used in k shortest path.
}

var ErrStop = x.Errorf("STOP")
var ErrTooBig = x.Errorf("Query exceeded memory limit. Please modify the query")
var ErrFacet = x.Errorf("Skip the edge")

type priorityQueue []*Item

func (h priorityQueue) Len() int           { return len(h) }
func (h priorityQueue) Less(i, j int) bool { return h[i].cost < h[j].cost }
func (h priorityQueue) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}
func (h *priorityQueue) Push(x interface{}) {
	n := len(*h)
	item := x.(*Item)
	item.index = n
	*h = append(*h, item)
}

func (h *priorityQueue) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	x.index = -1
	return x
}

type mapItem struct {
	attr  string
	cost  float64
	facet *protos.Facets
}

// We manintain a map from UID to nodeInfo for Djikstras.
type nodeInfo struct {
	mapItem
	parent uint64
	// Pointer to the item in heap. Used to update priority
	node *Item
}

func (sg *SubGraph) getCost(matrix, list int) (cost float64,
	fcs *protos.Facets, rerr error) {

	cost = 1.0
	if sg.Params.Facet == nil {
		return cost, fcs, rerr
	}
	fcsList := sg.facetsMatrix[matrix].FacetsList
	if len(fcsList) <= list {
		rerr = ErrFacet
		return cost, fcs, rerr
	}
	fcs = fcsList[list]
	if len(fcs.Facets) == 0 {
		rerr = ErrFacet
		return cost, fcs, rerr
	}
	if len(fcs.Facets) > 1 {
		rerr = x.Errorf("Expected 1 but got %d facets", len(fcs.Facets))
		return cost, fcs, rerr
	}
	tv := facets.ValFor(fcs.Facets[0])
	if tv.Tid == types.IntID {
		cost = float64(tv.Value.(int64))
	} else if tv.Tid == types.FloatID {
		cost = float64(tv.Value.(float64))
	} else {
		rerr = ErrFacet
	}
	return cost, fcs, rerr
}

func (start *SubGraph) expandOut(ctx context.Context,
	adjacencyMap map[uint64]map[uint64]mapItem, next chan bool, rch chan error) {

	var numEdges uint64
	var exec []*SubGraph
	var err error
	in := []uint64{start.Params.From}
	start.SrcUIDs = &protos.List{in}
	start.uidMatrix = []*protos.List{{in}}
	start.DestUIDs = start.SrcUIDs

	for _, child := range start.Children {
		child.SrcUIDs = start.DestUIDs
		exec = append(exec, child)
	}
	dummy := &SubGraph{}
	for {
		isNext := <-next
		if !isNext {
			return
		}
		rrch := make(chan error, len(exec))
		for _, sg := range exec {
			go ProcessGraph(ctx, sg, dummy, rrch)
		}

		for range exec {
			select {
			case err = <-rrch:
				if err != nil {
					x.TraceError(ctx, x.Wrapf(err, "Error while processing child task"))
					rch <- err
					return
				}
			case <-ctx.Done():
				x.TraceError(ctx, x.Wrapf(ctx.Err(), "Context done before full execution"))
				rch <- ctx.Err()
				return
			}
		}

		for _, sg := range exec {
			// Send the destuids in res chan.
			for mIdx, fromUID := range sg.SrcUIDs.Uids {
				for lIdx, toUID := range sg.uidMatrix[mIdx].Uids {
					if adjacencyMap[fromUID] == nil {
						adjacencyMap[fromUID] = make(map[uint64]mapItem)
					}
					// The default cost we'd use is 1.
					cost, facet, err := sg.getCost(mIdx, lIdx)
					if err == ErrFacet {
						// Ignore the edge and continue.
						continue
					} else if err != nil {
						rch <- err
						return
					}
					adjacencyMap[fromUID][toUID] = mapItem{
						cost:  cost,
						facet: facet,
						attr:  sg.Attr,
					}
					numEdges++
				}
			}
		}

		if numEdges > 10000000 {
			// If we've seen too many nodes, stop the query.
			rch <- ErrTooBig
			return
		}

		// modify the exec and attach child nodes.
		var out []*SubGraph
		for _, sg := range exec {
			if len(sg.DestUIDs.Uids) == 0 {
				continue
			}
			for _, child := range start.Children {
				temp := new(SubGraph)
				temp.copyFiltersRecurse(child)

				temp.SrcUIDs = sg.DestUIDs
				// Remove those nodes which we have already traversed. As this cannot be
				// in the path again.
				algo.ApplyFilter(temp.SrcUIDs, func(uid uint64, i int) bool {
					_, ok := adjacencyMap[uid]
					return !ok
				})
				if len(temp.SrcUIDs.Uids) == 0 {
					continue
				}
				sg.Children = append(sg.Children, temp)
				out = append(out, temp)
			}
		}

		if len(out) == 0 {
			rch <- ErrStop
			return
		}
		rch <- nil
		exec = out
	}
}

func (temp *SubGraph) copyFiltersRecurse(sg *SubGraph) {
	*temp = *sg
	temp.Children = []*SubGraph{}
	temp.Filters = []*SubGraph{}
	for _, fc := range sg.Filters {
		tempChild := new(SubGraph)
		tempChild.copyFiltersRecurse(fc)
		temp.Filters = append(temp.Filters, tempChild)
	}
}

// Djikstras algorithm pseudocode for reference.
//
//
// 1  function Dijkstra(Graph, source):
// 2      dist[source] ← 0                                    // Initialization
// 3
// 4      create vertex set Q
// 5
// 6      for each vertex v in Graph:
// 7          if v ≠ source
// 8              dist[v] ← INFINITY                          // Unknown distance from source to v
// 9              prev[v] ← UNDEFINED                         // Predecessor of v
// 10
// 11         Q.add_with_priority(v, dist[v])
// 12
// 13
// 14     while Q is not empty:                              // The main loop
// 15         u ← Q.extract_min()                            // Remove and return best vertex
// 16         for each neighbor v of u:                       // only v that is still in Q
// 17             alt = dist[u] + length(u, v)
// 18             if alt < dist[v]
// 19                 dist[v] ← alt
// 20                 prev[v] ← u
// 21                 Q.decrease_priority(v, alt)
// 22
// 23     return dist[], prev[]

// ShortestPath computes the k shortest paths in ascending
// order of weights. It returns a maximum of k paths.
func ShortestPath(ctx context.Context, sg *SubGraph) ([]*SubGraph, error) {
	var err error
	if sg.Params.Alias != "shortest" {
		return nil, x.Errorf("Invalid shortest path query")
	}

	numPaths := sg.Params.numPaths
	if numPaths == 0 {
		// Return 1 path by default.
		numPaths = 1
	}

	var kpaths []route
	pq := make(priorityQueue, 0)
	heap.Init(&pq)

	// Initialize and push the source node.
	srcNode := &Item{
		uid:  sg.Params.From,
		cost: 0,
		hop:  0,
		path: route{[]pathInfo{pathInfo{uid: sg.Params.From}}},
	}
	heap.Push(&pq, srcNode)

	numHops := -1
	next := make(chan bool, 2)
	expandErr := make(chan error, 2)
	adjacencyMap := make(map[uint64]map[uint64]mapItem)
	go sg.expandOut(ctx, adjacencyMap, next, expandErr)

	// In k shortest path we can't have this. We store the path till a node in every
	// node.
	// map to store the min cost and parent of nodes.
	var stopExpansion bool
	for pq.Len() > 0 {
		item := heap.Pop(&pq).(*Item)
		if item.uid == sg.Params.To {
			// Add path to list.
			kpaths = append(kpaths, item.path)
			if len(kpaths) == numPaths {
				// We found the required number of paths.
				break
			}
		}
		if item.hop > numHops {
			// Explore the next level by calling processGraph and add them
			// to the queue.
			if !stopExpansion {
				next <- true
				select {
				case err = <-expandErr:
					if err != nil {
						if err == ErrTooBig {
							return nil, err
						} else if err == ErrStop {
							stopExpansion = true
						} else {
							x.TraceError(ctx, x.Wrapf(err, "Error while processing child task"))
							return nil, err
						}
					}
				case <-ctx.Done():
					x.TraceError(ctx, x.Wrapf(ctx.Err(), "Context done before full execution"))
					return nil, ctx.Err()
				}
				numHops++
			}
		}
		if stopExpansion && numPaths == 1 {
			continue
		}
		neighbours := adjacencyMap[item.uid]
		for toUid, info := range neighbours {
			cost := info.cost
			curPath := make([]pathInfo, len(item.path.route)+1)
			n := copy(curPath, item.path.route)
			curPath[n] = pathInfo{
				uid:   toUid,
				attr:  info.attr,
				facet: info.facet,
			}
			node := &Item{
				uid:  toUid,
				cost: item.cost + cost,
				hop:  item.hop + 1,
				path: route{curPath},
			}
			heap.Push(&pq, node)
		}
	}

	next <- false

	if len(kpaths) == 0 {
		sg.DestUIDs = &protos.List{}
		return nil, nil
	}
	var res []uint64
	for _, it := range kpaths[0].route {
		res = append(res, it.uid)
	}
	sg.DestUIDs.Uids = res
	shortestSg := createKPathSubgraph(ctx, kpaths)
	return shortestSg, nil
}

func createKPathSubgraph(ctx context.Context, kpaths []route) []*SubGraph {
	var res []*SubGraph
	for _, it := range kpaths {
		shortestSg := new(SubGraph)
		shortestSg.Params = params{
			Alias:  "_path_",
			GetUid: true,
		}
		curUid := it.route[0].uid
		shortestSg.SrcUIDs = &protos.List{[]uint64{curUid}}
		shortestSg.DestUIDs = &protos.List{[]uint64{curUid}}
		shortestSg.uidMatrix = []*protos.List{{[]uint64{curUid}}}

		curNode := shortestSg
		i := 0
		for ; i < len(it.route)-1; i++ {
			curUid := it.route[i].uid
			childUid := it.route[i+1].uid
			node := new(SubGraph)
			node.Params = params{
				GetUid: true,
			}
			if it.route[i+1].facet != nil {
				// For consistent later processing.
				node.Params.Facet = &protos.Param{}
			}
			node.Attr = it.route[i+1].attr
			node.facetsMatrix = []*protos.FacetsList{{[]*protos.Facets{it.route[i+1].facet}}}
			node.SrcUIDs = &protos.List{[]uint64{curUid}}
			node.DestUIDs = &protos.List{[]uint64{childUid}}
			node.uidMatrix = []*protos.List{{[]uint64{childUid}}}

			curNode.Children = append(curNode.Children, node)
			curNode = node
		}

		node := new(SubGraph)
		node.Params = params{
			GetUid: true,
		}
		uid := it.route[i].uid
		node.SrcUIDs = &protos.List{[]uint64{uid}}
		node.uidMatrix = []*protos.List{{[]uint64{uid}}}
		curNode.Children = append(curNode.Children, node)

		res = append(res, shortestSg)
	}
	return res
}
