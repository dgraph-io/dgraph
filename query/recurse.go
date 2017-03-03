package query

import (
	"context"
	"fmt"
	"math"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/x"
)

func (start *SubGraph) expandRecurse(ctx context.Context,
	next chan bool, rch chan error) {

	// Note: Key format is - "attr|fromUID|toUID"
	reachMap := make(map[string]struct{})
	var numEdges int
	var exec []*SubGraph
	var err error

	rrch := make(chan error, len(exec))
	startChildren := make([]*SubGraph, len(start.Children))
	copy(startChildren, start.Children)
	start.Children = []*SubGraph{}

	// Process the root first.
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
		isNext := <-next
		if !isNext {
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
			for mIdx := -1; it.Valid(); it.Next() {
				mIdx++
				fromUID := it.Val()
				if len(sg.Filters) > 0 {
					// We need to do this in case we had some filters.
					algo.IntersectWith(sg.uidMatrix[mIdx], sg.DestUIDs)
				}
				algo.ApplyFilter(sg.uidMatrix[mIdx], func(uid uint64, i int) bool {
					key := fmt.Sprintf("%s|%d|%d", sg.Attr, fromUID, uid)
					_, ok := reachMap[key] // Combine fromUID here.
					return !ok
				})
			}
			sg.DestUIDs = algo.MergeSorted(sg.uidMatrix)
		}

		if numEdges > 1000000 {
			// If we've seen too many nodes, stop the query.
			rch <- ErrTooBig
			return
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
				temp.Children = []*SubGraph{}
				temp.SrcUIDs = sg.DestUIDs
				sg.Children = append(sg.Children, temp)
				out = append(out, temp)
			}
			// Mark the reached nodes
			it := algo.NewListIterator(sg.SrcUIDs)
			for mIdx := -1; it.Valid(); it.Next() {
				mIdx++
				fromUID := it.Val()
				toIt := algo.NewListIterator(sg.uidMatrix[mIdx])
				for ; toIt.Valid(); toIt.Next() {
					toUID := toIt.Val()
					key := fmt.Sprintf("%s|%d|%d", sg.Attr, fromUID, toUID)
					// Mark this edge as taken. We'd disallow this edge later.
					reachMap[key] = struct{}{}
					numEdges++
				}
			}

		}

		if len(out) == 0 {
			rch <- ErrStop
			return
		}
		// This marks the end of one level of exectution.
		rch <- nil
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
		// If no depth is specified, expand till we reach all leaf nodes
		// or we see reach too many nodes.
		depth = math.MaxUint64
	}

L:
	// Recurse number of times specified by the user.
	for i := uint64(0); i < depth; i++ {
		next <- true
		select {
		case err = <-expandErr:
			if err != nil {
				if err == ErrTooBig {
					return err
				}
				if err == ErrStop {
					break L
				}
				x.TraceError(ctx, x.Wrapf(err, "Error while processing child task"))
				return err
			}
		case <-ctx.Done():
			x.TraceError(ctx, x.Wrapf(ctx.Err(), "Context done before full execution"))
			return ctx.Err()
		}
	}
	// Done expanding.
	next <- false
	return nil
}
