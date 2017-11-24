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
	"math"
	"sync"

	"golang.org/x/net/trace"

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

var pathPool = sync.Pool{
	New: func() interface{} {
		return []pathInfo{}
	},
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
					if tr, ok := trace.FromContext(ctx); ok {
						tr.LazyPrintf("Error while processing child task: %+v", err)
					}
					rch <- err
					return
				}
			case <-ctx.Done():
				if tr, ok := trace.FromContext(ctx); ok {
					tr.LazyPrintf("Context done before full execution: %+v", ctx.Err())
				}
				rch <- ctx.Err()
				return
			}
		}

		for _, sg := range exec {
			select {
			case <-ctx.Done():
				if tr, ok := trace.FromContext(ctx); ok {
					tr.LazyPrintf("Context done before full execution: %+v", ctx.Err())
				}
				rch <- ctx.Err()
				return
			default:
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
			select {
			case <-ctx.Done():
				if tr, ok := trace.FromContext(ctx); ok {
					tr.LazyPrintf("Context done before full execution: %+v", ctx.Err())
				}
				rch <- ctx.Err()
				return
			default:
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

func KShortestPath(ctx context.Context, sg *SubGraph) ([]*SubGraph, error) {
	var err error
	if sg.Params.Alias != "shortest" {
		return nil, x.Errorf("Invalid shortest path query")
	}

	numPaths := sg.Params.numPaths
	var kroutes []route
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
	maxHops := int(sg.Params.ExploreDepth)
	isPossible := false
	if maxHops == 0 {
		maxHops = int(math.MaxInt32)
	}
	next := make(chan bool, 2)
	//cycles := 0
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
			kroutes = append(kroutes, item.path)
			if len(kroutes) == numPaths {
				// We found the required number of paths.
				break
			}
		}
		if item.hop > numHops && numHops < maxHops {
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
							if tr, ok := trace.FromContext(ctx); ok {
								tr.LazyPrintf("Error while processing child task: %+v", err)
							}
						} else {
							return nil, err
						}
					}
				case <-ctx.Done():
					if tr, ok := trace.FromContext(ctx); ok {
						tr.LazyPrintf("Context done before full execution: %+v", ctx.Err())
					}
					return nil, ctx.Err()
				}
				numHops++
			}
		}
		select {
		case <-ctx.Done():
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Context done before full execution: %+v", ctx.Err())
			}
			return nil, ctx.Err()
		default:
			if stopExpansion {
				// Allow loops once we have found one path.
				if !isPossible {
					continue
				}
			}
		}
		neighbours := adjacencyMap[item.uid]
		for toUid, info := range neighbours {
			cost := info.cost
			curPath := pathPool.Get().([]pathInfo)
			if cap(curPath) < len(item.path.route)+1 {
				// We can't use it due to insufficient capacity. Put it back.
				pathPool.Put(curPath)
				curPath = make([]pathInfo, len(item.path.route)+1)
			} else {
				// Use the curPath from pathPool. Set length appropriately.
				curPath = curPath[:len(item.path.route)+1]
			}
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
			if node.uid == sg.Params.To {
				isPossible = true
			}
			heap.Push(&pq, node)
		}
		// Return the popped nodes path to pool.
		pathPool.Put(item.path.route)
	}

	next <- false

	if len(kroutes) == 0 {
		sg.DestUIDs = &protos.List{}
		return nil, nil
	}
	var res []uint64
	for _, it := range kroutes[0].route {
		res = append(res, it.uid)
	}
	sg.DestUIDs.Uids = res
	shortestSg := createkroutesubgraph(ctx, kroutes)
	return shortestSg, nil
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

	if numPaths > 1 {
		return KShortestPath(ctx, sg)
	}
	pq := make(priorityQueue, 0)
	heap.Init(&pq)

	// Initialize and push the source node.
	srcNode := &Item{
		uid:  sg.Params.From,
		cost: 0,
		hop:  0,
	}
	heap.Push(&pq, srcNode)

	numHops := -1
	maxHops := int(sg.Params.ExploreDepth)
	if maxHops == 0 {
		maxHops = int(math.MaxInt32)
	}
	next := make(chan bool, 2)
	expandErr := make(chan error, 2)
	adjacencyMap := make(map[uint64]map[uint64]mapItem)
	go sg.expandOut(ctx, adjacencyMap, next, expandErr)

	// map to store the min cost and parent of nodes.
	dist := make(map[uint64]nodeInfo)
	dist[srcNode.uid] = nodeInfo{
		parent: 0,
		node:   srcNode,
		mapItem: mapItem{
			cost: 0,
		},
	}

	var stopExpansion bool
	for pq.Len() > 0 {
		item := heap.Pop(&pq).(*Item)
		if item.uid == sg.Params.To {
			break
		}
		if item.hop > numHops && numHops < maxHops {
			// Explore the next level by calling processGraph and add them
			// to the queue.
			if !stopExpansion {
				next <- true
			}
			select {
			case err = <-expandErr:
				if err != nil {
					if err == ErrTooBig {
						return nil, err
					} else if err == ErrStop {
						stopExpansion = true
					} else {
						if tr, ok := trace.FromContext(ctx); ok {
							tr.LazyPrintf("Error while processing child task: %+v", err)
						}
						return nil, err
					}
				}
			case <-ctx.Done():
				if tr, ok := trace.FromContext(ctx); ok {
					tr.LazyPrintf("Context done before full execution: %+v", ctx.Err())
				}
				return nil, ctx.Err()
			}
			numHops++
		}
		select {
		case <-ctx.Done():
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Context done before full execution: %+v", ctx.Err())
			}
			return nil, ctx.Err()
		default:
			if !stopExpansion {
				neighbours := adjacencyMap[item.uid]
				for toUid, info := range neighbours {
					cost := info.cost
					d, ok := dist[toUid]
					if ok && d.cost <= item.cost+cost {
						continue
					}
					if !ok {
						// This is the first time we're seeing this node. So
						// create a new node and add it to the heap and map.
						node := &Item{
							uid:  toUid,
							cost: item.cost + cost,
							hop:  item.hop + 1,
						}
						heap.Push(&pq, node)
						dist[toUid] = nodeInfo{
							parent: item.uid,
							node:   node,
							mapItem: mapItem{
								cost:  item.cost + cost,
								attr:  info.attr,
								facet: info.facet,
							},
						}
					} else {
						// We've already seen this node. So, just update the cost
						// and fix the priority in the heap and map.
						node := dist[toUid].node
						node.cost = item.cost + cost
						node.hop = item.hop + 1
						heap.Fix(&pq, node.index)
						// Update the map with new values.
						dist[toUid] = nodeInfo{
							parent: item.uid,
							node:   node,
							mapItem: mapItem{
								cost:  item.cost + cost,
								attr:  info.attr,
								facet: info.facet,
							},
						}
					}
				}
			}
		}
	}

	next <- false
	// Go through the distance map to find the path.
	var result []uint64
	cur := sg.Params.To
	for i := 0; cur != sg.Params.From && i < len(dist); i++ {
		result = append(result, cur)
		cur = dist[cur].parent
	}
	// Put the path in DestUIDs of the root.
	if cur != sg.Params.From {
		sg.DestUIDs = &protos.List{}
		return nil, nil
	}

	result = append(result, cur)
	l := len(result)
	// Reverse the list.
	for i := 0; i < l/2; i++ {
		result[i], result[l-i-1] = result[l-i-1], result[i]
	}
	sg.DestUIDs.Uids = result

	shortestSg := createPathSubgraph(ctx, dist, result)
	return []*SubGraph{shortestSg}, nil
}

func createPathSubgraph(ctx context.Context, dist map[uint64]nodeInfo, result []uint64) *SubGraph {
	shortestSg := new(SubGraph)
	shortestSg.Params = params{
		Alias:    "_path_",
		shortest: true,
	}
	curUid := result[0]
	shortestSg.SrcUIDs = &protos.List{[]uint64{curUid}}
	shortestSg.DestUIDs = &protos.List{[]uint64{curUid}}
	shortestSg.uidMatrix = []*protos.List{{[]uint64{curUid}}}

	curNode := shortestSg
	for i := 0; i < len(result)-1; i++ {
		curUid := result[i]
		childUid := result[i+1]
		node := new(SubGraph)
		nodeInfo := dist[childUid]
		node.Params = params{
			shortest: true,
		}
		if nodeInfo.facet != nil {
			// For consistent later processing.
			node.Params.Facet = &protos.FacetParams{}
		}
		node.Attr = nodeInfo.attr
		node.facetsMatrix = []*protos.FacetsList{{[]*protos.Facets{nodeInfo.facet}}}
		node.SrcUIDs = &protos.List{[]uint64{curUid}}
		node.DestUIDs = &protos.List{[]uint64{childUid}}
		node.uidMatrix = []*protos.List{{[]uint64{childUid}}}

		curNode.Children = append(curNode.Children, node)
		curNode = node
	}

	node := new(SubGraph)
	node.Params = params{
		shortest: true,
	}
	uid := result[len(result)-1]
	node.SrcUIDs = &protos.List{[]uint64{uid}}
	node.uidMatrix = []*protos.List{{[]uint64{uid}}}
	curNode.Children = append(curNode.Children, node)

	return shortestSg
}

func createkroutesubgraph(ctx context.Context, kroutes []route) []*SubGraph {
	var res []*SubGraph
	for _, it := range kroutes {
		shortestSg := new(SubGraph)
		shortestSg.Params = params{
			Alias:    "_path_",
			shortest: true,
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
				shortest: true,
			}
			if it.route[i+1].facet != nil {
				// For consistent later processing.
				node.Params.Facet = &protos.FacetParams{}
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
			shortest: true,
		}
		uid := it.route[i].uid
		node.SrcUIDs = &protos.List{[]uint64{uid}}
		node.uidMatrix = []*protos.List{{[]uint64{uid}}}
		curNode.Children = append(curNode.Children, node)

		res = append(res, shortestSg)
	}
	return res
}
