package query

import (
	"context"
	"math"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/x"
)

func (start *SubGraph) expandRecurse(ctx context.Context,
	next chan bool, rch chan error) {

	reachMap := make(map[uint64]struct{})
	var numEdges int
	var exec []*SubGraph
	var err error

	// Process the root first.
	rrch := make(chan error, len(exec))
	startChildren := make([]*SubGraph, len(start.Children))
	copy(startChildren, start.Children)
	start.Children = []*SubGraph{}
	go ProcessGraph(ctx, start, nil, rrch)
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

	// Prepare the children.
	for _, child := range startChildren {
		temp := new(SubGraph)
		*temp = *child
		temp.SrcUIDs = start.DestUIDs
		temp.Children = []*SubGraph{}
		exec = append(exec, temp)
		start.Children = append(start.Children, temp)
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
			it := algo.NewListIterator(sg.SrcUIDs)
			mIdx := -1
			for ; it.Valid(); it.Next() { // idx, fromUID := range sg.SrcUIDs.Uids {
				mIdx++
				fromUID := it.Val()
				if l := algo.ListLen(sg.uidMatrix[mIdx]); l > 0 {
					// Mark as set only if its not a value edge.
					reachMap[fromUID] = struct{}{}
					numEdges += l
				}
			}
		}

		if numEdges > 1000000 {
			// If we've seen too many nodes, stop the query.
			rch <- ErrTooBig
		}

		// modify the exec and attach child nodes.
		var out []*SubGraph
		for _, sg := range exec {
			if algo.ListLen(sg.DestUIDs) == 0 {
				continue
			}
			for _, child := range startChildren {
				temp := new(SubGraph)
				*temp = *child
				temp.SrcUIDs = sg.DestUIDs
				temp.Children = []*SubGraph{}
				// Remove those nodes which we have already traversed. As this cannot be
				// in the path again.
				algo.ApplyFilter(temp.SrcUIDs, func(uid uint64, i int) bool {
					_, ok := reachMap[uid]
					return !ok
				})
				if algo.ListLen(temp.SrcUIDs) == 0 {
					continue
				}
				sg.Children = append(sg.Children, temp)
				out = append(out, temp)
			}
		}

		if len(out) == 0 {
			rch <- ErrStop
		} else {
			rch <- nil
		}
		exec = out
	}
}

func Recurse(ctx context.Context, sg *SubGraph) error {
	var err error
	if sg.Params.Alias != "recurse" {
		return x.Errorf("Invalid shortest path query")
	}
	expandErr := make(chan error, 2)
	next := make(chan bool, 2)
	go sg.expandRecurse(ctx, next, expandErr)
	depth := sg.Params.RecurseDepth
	if depth == 0 {
		depth = math.MaxInt64
	}

L:
	// Recurse number of times specified by the user.
	for i := 0; i < depth; i++ {
		next <- false
		select {
		case err = <-expandErr:
			if err != nil {
				if err == ErrTooBig {
					return err
				} else if err == ErrStop {
					break L
				} else {
					x.TraceError(ctx, x.Wrapf(err, "Error while processing child task"))
					return err
				}
			}
		case <-ctx.Done():
			x.TraceError(ctx, x.Wrapf(ctx.Err(), "Context done before full execution"))
			return ctx.Err()
		}
	}
	// Done expanding.
	next <- true
	return nil
}
