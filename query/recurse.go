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
	"context"
	"fmt"
	"math"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/x"
)

func (start *SubGraph) expandRecurse(ctx context.Context, maxDepth uint64) error {
	// Note: Key format is - "attr|fromUID|toUID"
	reachMap := make(map[string]struct{})
	var numEdges int
	var exec []*SubGraph
	var err error

	rrch := make(chan error, len(exec))
	startChildren := make([]*SubGraph, len(start.Children))
	copy(startChildren, start.Children)
	// Empty children before giving to ProcessGraph as we are only concerned with DestUids.
	start.Children = start.Children[:0]

	// Process the root first.
	go ProcessGraph(ctx, start, nil, rrch)
	select {
	case err = <-rrch:
		if err != nil {
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Error while processing child task: %+v", err)
			}
			return err
		}
	case <-ctx.Done():
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Context done before full execution: %+v", ctx.Err())
		}
		return ctx.Err()
	}

	// Add children back so that expandSubgraph can expand them if needed.
	start.Children = append(start.Children, startChildren...)
	if startChildren, err = expandSubgraph(ctx, start); err != nil {
		return err
	}

	start.Children = start.Children[:0]
	for _, child := range startChildren {
		temp := new(SubGraph)
		temp.copyFiltersRecurse(child)
		temp.SrcUIDs = start.DestUIDs
		temp.Params.Var = child.Params.Var
		exec = append(exec, temp)
		start.Children = append(start.Children, temp)
	}

	dummy := &SubGraph{}
	var depth uint64
	for {
		if depth >= maxDepth {
			return nil
		}
		depth++

		rrch := make(chan error, len(exec))
		for _, sg := range exec {
			go ProcessGraph(ctx, sg, dummy, rrch)
		}

		var recurseErr error
		for range exec {
			select {
			case err = <-rrch:
				if err != nil {
					if tr, ok := trace.FromContext(ctx); ok {
						tr.LazyPrintf("Error while processing child task: %+v", err)
					}
					if recurseErr == nil {
						recurseErr = err
					}
				}
			case <-ctx.Done():
				if tr, ok := trace.FromContext(ctx); ok {
					tr.LazyPrintf("Context done before full execution: %+v", ctx.Err())
				}
				if recurseErr == nil {
					recurseErr = ctx.Err()
				}
			}
		}

		if recurseErr != nil {
			return recurseErr
		}

		for _, sg := range exec {
			if len(sg.Filters) > 0 {
				// We need to do this in case we had some filters.
				sg.updateUidMatrix()
			}
			for mIdx, fromUID := range sg.SrcUIDs.Uids {
				// This is for avoiding loops in graph.
				algo.ApplyFilter(sg.uidMatrix[mIdx], func(uid uint64, i int) bool {
					key := fmt.Sprintf("%s|%d|%d", sg.Attr, fromUID, uid)
					_, seen := reachMap[key] // Combine fromUID here.
					if seen {
						return false
					} else {
						// Mark this edge as taken. We'd disallow this edge later.
						reachMap[key] = struct{}{}
						numEdges++
						return true
					}
				})
			}
			if len(sg.Params.Order) > 0 || len(sg.Params.FacetOrder) > 0 {
				// Can't use merge sort if the UIDs are not sorted.
				sg.updateDestUids()
			} else {
				sg.DestUIDs = algo.MergeSorted(sg.uidMatrix)
			}
		}

		// modify the exec and attach child nodes.
		var out []*SubGraph
		for _, sg := range exec {
			if len(sg.DestUIDs.Uids) == 0 {
				continue
			}
			for _, child := range startChildren {
				temp := new(SubGraph)
				temp.copyFiltersRecurse(child)
				temp.SrcUIDs = sg.DestUIDs
				temp.Params.Var = child.Params.Var
				sg.Children = append(sg.Children, temp)
				out = append(out, temp)
			}
		}

		if numEdges > 1000000 {
			// If we've seen too many nodes, stop the query.
			return ErrTooBig
		}

		if len(out) == 0 {
			return nil
		}
		exec = out
	}
}

func Recurse(ctx context.Context, sg *SubGraph) error {
	if !sg.Params.Recurse {
		return x.Errorf("Invalid recurse path query")
	}

	depth := sg.Params.ExploreDepth
	if depth == 0 {
		// If no depth is specified, expand till we reach all leaf nodes
		// or we see reach too many nodes.
		depth = math.MaxUint64
	}
	return sg.expandRecurse(ctx, depth)
}
