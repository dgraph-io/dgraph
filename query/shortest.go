package query

import (
	"container/heap"
	"context"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

type Item struct {
	uid  uint64  // uid of the node.
	cost float64 // cost of taking the path till this uid.
	hop  int     // number of hops taken to reach this node.
}

var ErrStop = x.Errorf("STOP")
var ErrTooBig = x.Errorf("Query exceeded memory limit")

type priorityQueue []*Item

func (h priorityQueue) Len() int           { return len(h) }
func (h priorityQueue) Less(i, j int) bool { return h[i].cost < h[j].cost }
func (h priorityQueue) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}
func (h *priorityQueue) Push(x interface{}) {
	item := x.(*Item)
	*h = append(*h, item)
}

func (h *priorityQueue) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

type minInfo struct {
	cost   float64
	parent uint64
}

func (start *SubGraph) expandOut(ctx context.Context,
	mp map[uint64]map[uint64]float64, next chan bool, rch chan error) {

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

		for _, sg := range exec {
			// Send the destuids in res chan.
			for idx, fromUID := range sg.SrcUIDs.Uids {
				ul := sg.uidMatrix[idx].Uids
				for _, toUid := range ul {
					if mp[fromUID] == nil {
						mp[fromUID] = make(map[uint64]float64)
					}
					mp[fromUID][toUid] = 1.0 // cost is 1 for now.
				}
			}
		}

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
				// Remove those nodes which we have already traversed. As this cannot be
				// in the path again.
				algo.ApplyFilter(temp.SrcUIDs, func(uid uint64, i int) bool {
					_, ok := mp[uid]
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
		}
		rch <- nil
		exec = out
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

func ShortestPath(ctx context.Context, sg *SubGraph) error {
	var err error
	if sg.Params.Alias != "shortest" {
		return x.Errorf("Invalid shortest path query")
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
	next := make(chan bool, 2)
	expandErr := make(chan error, 2)
	mp := make(map[uint64]map[uint64]float64)
	go sg.expandOut(ctx, mp, next, expandErr)

	// map to store the min cost and parent of nodes.
	dist := make(map[uint64]minInfo)
	dist[srcNode.uid] = minInfo{
		parent: 0,
		cost:   0,
	}

	var stopExpansion bool
	// For now, lets allow a maximum of 10 hops.
	for pq.Len() > 0 && len(mp) < 1000000 {
		item := heap.Pop(&pq).(*Item)
		if item.uid == sg.Params.To {
			break
		}
		if item.hop > numHops {
			// Explore the next level by calling processGraph and add them
			// to the queue.
			if !stopExpansion {
				next <- false
			}
			select {
			case err = <-expandErr:
				if err != nil {
					if err == ErrTooBig {
						return err
					} else if err == ErrStop {
						stopExpansion = true
					} else {
						x.TraceError(ctx, x.Wrapf(err, "Error while processing child task"))
						return err
					}
				}
			case <-ctx.Done():
				x.TraceError(ctx, x.Wrapf(ctx.Err(), "Context done before full execution"))
				return ctx.Err()
			}
			numHops++
		}
		if !stopExpansion {
			neighbours := mp[item.uid]
			for toUid, cost := range neighbours {
				if d, ok := dist[toUid]; !ok || d.cost > item.cost+cost {
					node := &Item{
						uid:  toUid,
						cost: item.cost + cost,
						hop:  item.hop + 1,
					}
					heap.Push(&pq, node) // Add a node with lesser cost in the queue.
					dist[toUid] = minInfo{
						cost:   item.cost + cost,
						parent: item.uid,
					}
				}
			}
		}
	}

	// Go through the distance map to find the path.
	result := new(task.List)
	cur := sg.Params.To
	for i := 0; cur != sg.Params.From && i < len(dist); i++ {
		result.Uids = append(result.Uids, cur)
		cur = dist[cur].parent
	}
	// Put the path in DestUIDs of the root.
	if cur == sg.Params.From {
		result.Uids = append(result.Uids, cur)
		l := len(result.Uids)
		// Reverse the list.
		for i := 0; i < l/2; i++ {
			result.Uids[i], result.Uids[l-i-1] = result.Uids[l-i-1], result.Uids[i]
		}
		sg.DestUIDs = result
	} else {
		sg.DestUIDs = &task.List{}
	}
	next <- true
	return nil
}
