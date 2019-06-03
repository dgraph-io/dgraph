/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package query

import (
	"container/heap"
	"context"
	"math"
	"sync"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

type pathInfo struct {
	uid   uint64
	attr  string
	facet *pb.Facets
}

type route struct {
	route       []pathInfo
	totalWeight float64
}

type queueItem struct {
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

var errStop = errors.Errorf("STOP")
var errFacet = errors.Errorf("Skip the edge")

type priorityQueue []*queueItem

func (h priorityQueue) Len() int           { return len(h) }
func (h priorityQueue) Less(i, j int) bool { return h[i].cost < h[j].cost }
func (h priorityQueue) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}
func (h *priorityQueue) Push(x interface{}) {
	n := len(*h)
	item := x.(*queueItem)
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
	facet *pb.Facets
}

// We manintain a map from UID to nodeInfo for Djikstras.
type nodeInfo struct {
	mapItem
	parent uint64
	// Pointer to the item in heap. Used to update priority
	node *queueItem
}

func (sg *SubGraph) getCost(matrix, list int) (cost float64,
	fcs *pb.Facets, rerr error) {

	cost = 1.0
	if len(sg.facetsMatrix) <= matrix {
		return cost, fcs, rerr
	}
	fcsList := sg.facetsMatrix[matrix].FacetsList
	if len(fcsList) <= list {
		rerr = errFacet
		return cost, fcs, rerr
	}
	fcs = fcsList[list]
	if len(fcs.Facets) == 0 {
		rerr = errFacet
		return cost, fcs, rerr
	}
	if len(fcs.Facets) > 1 {
		rerr = errors.Errorf("Expected 1 but got %d facets", len(fcs.Facets))
		return cost, fcs, rerr
	}
	tv, err := facets.ValFor(fcs.Facets[0])
	if err != nil {
		return 0.0, nil, err
	}
	if tv.Tid == types.IntID {
		cost = float64(tv.Value.(int64))
	} else if tv.Tid == types.FloatID {
		cost = float64(tv.Value.(float64))
	} else {
		rerr = errFacet
	}
	return cost, fcs, rerr
}

func (sg *SubGraph) expandOut(ctx context.Context,
	adjacencyMap map[uint64]map[uint64]mapItem, next chan bool, rch chan error) {

	var numEdges uint64
	var exec []*SubGraph
	var err error
	in := []uint64{sg.Params.From}
	sg.SrcUIDs = &pb.List{Uids: in}
	sg.uidMatrix = []*pb.List{{Uids: in}}
	sg.DestUIDs = sg.SrcUIDs

	for _, child := range sg.Children {
		child.SrcUIDs = sg.DestUIDs
		exec = append(exec, child)
	}
	dummy := &SubGraph{}
	for {
		isNext := <-next
		if !isNext {
			return
		}
		rrch := make(chan error, len(exec))
		for _, subgraph := range exec {
			go ProcessGraph(ctx, subgraph, dummy, rrch)
		}

		for range exec {
			select {
			case err = <-rrch:
				if err != nil {
					rch <- err
					return
				}
			case <-ctx.Done():
				rch <- ctx.Err()
				return
			}
		}

		for _, subgraph := range exec {
			select {
			case <-ctx.Done():
				rch <- ctx.Err()
				return
			default:
				if subgraph.UnknownAttr {
					continue
				}

				// Send the destuids in res chan.
				for mIdx, fromUID := range subgraph.SrcUIDs.Uids {
					for lIdx, toUID := range subgraph.uidMatrix[mIdx].Uids {
						if adjacencyMap[fromUID] == nil {
							adjacencyMap[fromUID] = make(map[uint64]mapItem)
						}
						// The default cost we'd use is 1.
						cost, facet, err := subgraph.getCost(mIdx, lIdx)
						if err == errFacet {
							// Ignore the edge and continue.
							continue
						} else if err != nil {
							rch <- err
							return
						}
						adjacencyMap[fromUID][toUID] = mapItem{
							cost:  cost,
							facet: facet,
							attr:  subgraph.Attr,
						}
						numEdges++
					}
				}
			}
		}

		if numEdges > x.Config.QueryEdgeLimit {
			// If we've seen too many edges, stop the query.
			rch <- errors.Errorf("Exceeded query edge limit = %v. Found %v edges.",
				x.Config.QueryEdgeLimit, numEdges)
			return
		}

		// modify the exec and attach child nodes.
		var out []*SubGraph
		for _, subgraph := range exec {
			if len(subgraph.DestUIDs.Uids) == 0 {
				continue
			}
			select {
			case <-ctx.Done():
				rch <- ctx.Err()
				return
			default:
				for _, child := range sg.Children {
					temp := new(SubGraph)
					temp.copyFiltersRecurse(child)

					temp.SrcUIDs = subgraph.DestUIDs
					// Remove those nodes which we have already traversed. As this cannot be
					// in the path again.
					algo.ApplyFilter(temp.SrcUIDs, func(uid uint64, i int) bool {
						_, ok := adjacencyMap[uid]
						return !ok
					})
					subgraph.Children = append(subgraph.Children, temp)
					out = append(out, temp)
				}
			}
		}

		if len(out) == 0 {
			rch <- errStop
			return
		}
		rch <- nil
		exec = out
	}
}

func (sg *SubGraph) copyFiltersRecurse(otherSubgraph *SubGraph) {
	*sg = *otherSubgraph
	sg.Children = []*SubGraph{}
	sg.Filters = []*SubGraph{}
	for _, fc := range otherSubgraph.Filters {
		tempChild := new(SubGraph)
		tempChild.copyFiltersRecurse(fc)
		sg.Filters = append(sg.Filters, tempChild)
	}
}

func runKShortestPaths(ctx context.Context, sg *SubGraph) ([]*SubGraph, error) {
	var err error
	if sg.Params.Alias != "shortest" {
		return nil, errors.Errorf("Invalid shortest path query")
	}

	numPaths := sg.Params.numPaths
	var kroutes []route
	pq := make(priorityQueue, 0)
	heap.Init(&pq)

	// Initialize and push the source node.
	srcNode := &queueItem{
		uid:  sg.Params.From,
		cost: 0,
		hop:  0,
		path: route{route: []pathInfo{{uid: sg.Params.From}}},
	}
	heap.Push(&pq, srcNode)

	numHops := -1
	maxHops := int(sg.Params.ExploreDepth)
	if maxHops == 0 {
		maxHops = int(math.MaxInt32)
	}
	minWeight := sg.Params.MinWeight
	maxWeight := sg.Params.MaxWeight
	next := make(chan bool, 2)
	expandErr := make(chan error, 2)
	adjacencyMap := make(map[uint64]map[uint64]mapItem)
	go sg.expandOut(ctx, adjacencyMap, next, expandErr)

	// In k shortest path we can't have this. We store the path till a node in every
	// node.
	// map to store the min cost and parent of nodes.
	var stopExpansion bool
	for pq.Len() > 0 {
		item := heap.Pop(&pq).(*queueItem)
		if item.uid == sg.Params.To {
			// Ignore paths that do not meet the minimum weight requirement.
			if item.cost < minWeight {
				continue
			}

			// Add path to list.
			newRoute := item.path
			newRoute.totalWeight = item.cost
			kroutes = append(kroutes, newRoute)
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
						if err == errStop {
							stopExpansion = true
						} else {
							return nil, err
						}
					}
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				numHops++
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			if stopExpansion {
				continue
			}
		}
		neighbours := adjacencyMap[item.uid]
		for toUid, info := range neighbours {
			cost := info.cost
			// Skip neighbour if the cost is greater than the maximum weight allowed.
			if item.cost+cost > maxWeight {
				continue
			}

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
			node := &queueItem{
				uid:  toUid,
				cost: item.cost + cost,
				hop:  item.hop + 1,
				path: route{route: curPath},
			}
			heap.Push(&pq, node)
		}
		// Return the popped nodes path to pool.
		pathPool.Put(item.path.route)
	}

	next <- false

	if len(kroutes) == 0 {
		sg.DestUIDs = &pb.List{}
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
func shortestPath(ctx context.Context, sg *SubGraph) ([]*SubGraph, error) {
	var err error
	if sg.Params.Alias != "shortest" {
		return nil, errors.Errorf("Invalid shortest path query")
	}
	numPaths := sg.Params.numPaths
	if numPaths == 0 {
		// Return 1 path by default.
		numPaths = 1
	}

	if numPaths > 1 {
		return runKShortestPaths(ctx, sg)
	}
	pq := make(priorityQueue, 0)
	heap.Init(&pq)

	// Initialize and push the source node.
	srcNode := &queueItem{
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
	var totalWeight float64
	for pq.Len() > 0 {
		item := heap.Pop(&pq).(*queueItem)
		if item.uid == sg.Params.To {
			totalWeight = item.cost
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
					if err == errStop {
						stopExpansion = true
					} else {
						return nil, err
					}
				}
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			numHops++
		}
		select {
		case <-ctx.Done():
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
						node := &queueItem{
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
		sg.DestUIDs = &pb.List{}
		return nil, nil
	}

	result = append(result, cur)
	l := len(result)
	// Reverse the list.
	for i := 0; i < l/2; i++ {
		result[i], result[l-i-1] = result[l-i-1], result[i]
	}
	sg.DestUIDs.Uids = result

	shortestSg := createPathSubgraph(ctx, dist, totalWeight, result)
	return []*SubGraph{shortestSg}, nil
}

func createPathSubgraph(ctx context.Context, dist map[uint64]nodeInfo, totalWeight float64,
	result []uint64) *SubGraph {
	shortestSg := new(SubGraph)
	shortestSg.Params = params{
		Alias:    "_path_",
		shortest: true,
	}
	shortestSg.pathMeta = &pathMetadata{
		weight: totalWeight,
	}
	curUid := result[0]
	shortestSg.SrcUIDs = &pb.List{Uids: []uint64{curUid}}
	shortestSg.DestUIDs = &pb.List{Uids: []uint64{curUid}}
	shortestSg.uidMatrix = []*pb.List{{Uids: []uint64{curUid}}}

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
			node.Params.Facet = &pb.FacetParams{}
		}
		node.Attr = nodeInfo.attr
		node.facetsMatrix = []*pb.FacetsList{{FacetsList: []*pb.Facets{nodeInfo.facet}}}
		node.SrcUIDs = &pb.List{Uids: []uint64{curUid}}
		node.DestUIDs = &pb.List{Uids: []uint64{childUid}}
		node.uidMatrix = []*pb.List{{Uids: []uint64{childUid}}}

		curNode.Children = append(curNode.Children, node)
		curNode = node
	}

	node := new(SubGraph)
	node.Params = params{
		shortest: true,
	}
	uid := result[len(result)-1]
	node.SrcUIDs = &pb.List{Uids: []uint64{uid}}
	node.uidMatrix = []*pb.List{{Uids: []uint64{uid}}}
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
		shortestSg.pathMeta = &pathMetadata{
			weight: it.totalWeight,
		}
		curUid := it.route[0].uid
		shortestSg.SrcUIDs = &pb.List{Uids: []uint64{curUid}}
		shortestSg.DestUIDs = &pb.List{Uids: []uint64{curUid}}
		shortestSg.uidMatrix = []*pb.List{{Uids: []uint64{curUid}}}

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
				node.Params.Facet = &pb.FacetParams{}
			}
			node.Attr = it.route[i+1].attr
			node.facetsMatrix = []*pb.FacetsList{{FacetsList: []*pb.Facets{it.route[i+1].facet}}}
			node.SrcUIDs = &pb.List{Uids: []uint64{curUid}}
			node.DestUIDs = &pb.List{Uids: []uint64{childUid}}
			node.uidMatrix = []*pb.List{{Uids: []uint64{childUid}}}

			curNode.Children = append(curNode.Children, node)
			curNode = node
		}

		node := new(SubGraph)
		node.Params = params{
			shortest: true,
		}
		uid := it.route[i].uid
		node.SrcUIDs = &pb.List{Uids: []uint64{uid}}
		node.uidMatrix = []*pb.List{{Uids: []uint64{uid}}}
		curNode.Children = append(curNode.Children, node)

		res = append(res, shortestSg)
	}
	return res
}
