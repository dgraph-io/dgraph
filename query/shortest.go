package query

import (
	"container/heap"
	"context"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

type Item struct {
	uid   uint64  // uid of the node.
	cost  float64 // cost of taking the path till this uid.
	hop   int     // number of hops taken to reach this node.
	index int
}

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
	return x
}

type info struct {
	from uint64
	to   uint64
	cost float64
}

func (start *SubGraph) expandOut(ctx context.Context, mp map[uint64]map[uint64]float64, next chan bool, rch chan error) {
	var exec []*SubGraph
	var err error
	start.SrcUIDs = &task.List{[]uint64{start.Params.From}}
	start.uidMatrix = []*task.List{&task.List{Uids: []uint64{start.Params.From}}}
	start.DestUIDs = start.SrcUIDs

	for _, child := range start.Children {
		child.SrcUIDs = start.DestUIDs
		exec = append(exec, child)
	}
	dummy := &SubGraph{}
	for {
		over := <-next
		if over {
			return
		}
		rrch := make(chan error, len(exec))
		for _, sg := range exec {
			go ProcessGraph(ctx, sg, dummy, rrch)
		}

		for _ = range exec {
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

		var isNew bool
		for _, sg := range exec {
			// Send the destuids in res chan.
			for idx, fromUID := range sg.SrcUIDs.Uids {
				ul := sg.uidMatrix[idx].Uids
				for _, toUid := range ul {
					// res <- info{fromUID, toUid, 1} // Cost is 1 for now.
					if mp[fromUID] == nil {
						mp[fromUID] = make(map[uint64]float64)
					}
					if _, ok := mp[fromUID][toUid]; !ok {
						isNew = true
					}
					mp[fromUID][toUid] = 1.0 // cost is 1 for now.
				}
			}
		}

		if !isNew {
			// No progress. So stop this function and notify the listner.
			rch <- x.Errorf("Stop Expansion")
			return
		}

		rch <- nil

		// modify the exec and attach child nodes.
		var out []*SubGraph
		for _, sg := range exec {
			if len(sg.DestUIDs.Uids) == 0 {
				continue
			}
			for _, child := range start.Children {
				temp := new(SubGraph)
				*temp = *child
				// Filter out the uids that we have already seen
				temp.Children = []*SubGraph{}
				temp.SrcUIDs = sg.DestUIDs
				/*
					// Remove those nodes which we have already traversed. Is it right?
					algo.ApplyFilter(temp.SrcUIDs, func(uid uint64, i int) bool {
						_, ok := mp[uid]
						return !ok
					})
				*/
				sg.Children = append(sg.Children, temp)
				out = append(out, temp)
			}
		}

		exec = out
	}
}

func ShortestPath(ctx context.Context, sg *SubGraph, rch chan error) {
	var err error
	if sg.Params.Alias != "shortest" {
		rch <- x.Errorf("Invalid shortest path query")
		return
	}

	pq := make(priorityQueue, 0)
	heap.Init(&pq)

	srcNode := &Item{
		uid:   sg.Params.From,
		cost:  0,
		hop:   0,
		index: 0,
	}

	heap.Push(&pq, srcNode)

	var finalCost float64
	numHops := -1

	next := make(chan bool, 2)
	rch1 := make(chan error, 2)
	mp := make(map[uint64]map[uint64]float64)
	go sg.expandOut(ctx, mp, next, rch1)
	dist := make(map[uint64]float64) // map to store the min cost to nodes we've seen.
	dist[srcNode.uid] = 0
	var stopExpansion bool

	//TODO(Ashwin): We can maintain another parent map which could avoid the tree traversal
	// later and let us find the path directly.

	// For now, lets allow a maximum of 10 hops.
	for pq.Len() > 0 && numHops < 10 {
		item := heap.Pop(&pq).(*Item)
		/*
			if item.hop > maxLoopLevel {
				break
			}
		*/
		if item.uid == sg.Params.To {
			finalCost = item.cost
			break
		}
		if item.hop > numHops {
			// Explore the next level by calling processGraph and add them
			// to the queue.
			if !stopExpansion {
				next <- false
			}

			select {
			case err = <-rch1:
				if err != nil {
					if err.Error() == "Stop Expansion" {
						stopExpansion = true
					} else {
						x.TraceError(ctx, x.Wrapf(err, "Error while processing child task"))
						rch <- err
						return
					}
				}
			case <-ctx.Done():
				x.TraceError(ctx, x.Wrapf(ctx.Err(), "Context done before full execution"))
				rch <- ctx.Err()
				return
			}

			numHops++
		}
		neigh := mp[item.uid]
		for toUid, cost := range neigh {
			if d, ok := dist[toUid]; !ok || d > item.cost+cost {
				node := &Item{toUid, item.cost + cost, item.hop + 1, 0}
				heap.Push(&pq, node) // Add a node with lesser cost in the queue.
				dist[toUid] = item.cost + cost
			}
		}
	}

	// Go through the execution tree to find the path.
	result := new(task.List)
	isPathFound := sg.getPath(sg.Params.From, sg.Params.To, result, 0, finalCost)
	if isPathFound {
		// Append the start node to the list.
		result.Uids = append(result.Uids, sg.Params.From)
		l := len(result.Uids)
		// Reverse the list.
		for i := 0; i < l/2; i++ {
			result.Uids[i], result.Uids[l-i-1] = result.Uids[l-i-1], result.Uids[i]
		}
	}

	// Put the path in DestUIDs of the root.
	sg.DestUIDs = result
	next <- true
	rch <- nil
}

func (sg *SubGraph) getPath(uid, to uint64, path *task.List, cost, finalCost float64) bool {
	if uid == to && cost == finalCost {
		// We found the required end node.
		return true
	}

	for _, pc := range sg.Children {
		idx := algo.IndexOf(pc.SrcUIDs, uid)
		if idx < 0 {
			continue
		}
		if len(pc.uidMatrix) <= idx {
			// Its possible that we created a child level but never executed it.
			return false
		}
		ul := pc.uidMatrix[idx]

		for _, childUID := range ul.Uids {
			if pc.getPath(childUID, to, path, cost+1, finalCost) {
				// If this node was on the path, add it to the list.
				path.Uids = append(path.Uids, childUID)
				return true
			}
		}
	}

	return false
}
