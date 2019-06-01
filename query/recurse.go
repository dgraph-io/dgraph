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
	"context"
	"fmt"
	"math"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

func (start *SubGraph) expandRecurse(ctx context.Context, maxDepth uint64) error {
	// Note: Key format is - "attr|fromUID|toUID"
	reachMap := make(map[string]struct{})
	allowLoop := start.Params.RecurseArgs.AllowLoop
	var numEdges uint64
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
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	if start.UnknownAttr {
		return nil
	}

	// Add children back and expand if necessary
	if exec, err = expandChildren(ctx, start, startChildren); err != nil {
		return err
	}

	dummy := &SubGraph{}
	var depth uint64
	for {
		if depth >= maxDepth {
			return nil
		}
		depth++

		// When the maximum depth has been reached, avoid retrieving any facets as
		// the nodes at the other end of the edge will not be a part of this query.
		// Otherwise, the facets will be included in the query without any other
		// information about the node, which is quite counter-intuitive.
		if depth == maxDepth {
			for _, sg := range exec {
				sg.Params.Facet = nil
			}
		}

		rrch := make(chan error, len(exec))
		for _, sg := range exec {
			go ProcessGraph(ctx, sg, dummy, rrch)
		}

		var recurseErr error
		for range exec {
			select {
			case err = <-rrch:
				if err != nil {
					if recurseErr == nil {
						recurseErr = err
					}
				}
			case <-ctx.Done():
				if recurseErr == nil {
					recurseErr = ctx.Err()
				}
			}
		}

		if recurseErr != nil {
			return recurseErr
		}

		for _, sg := range exec {
			// sg.uidMatrix can be empty. Continue if that is the case.
			if len(sg.uidMatrix) == 0 {
				continue
			}

			if sg.UnknownAttr {
				continue
			}

			if len(sg.Filters) > 0 {
				// We need to do this in case we had some filters.
				sg.updateUidMatrix()
			}

			for mIdx, fromUID := range sg.SrcUIDs.Uids {
				if allowLoop {
					for _, ul := range sg.uidMatrix {
						numEdges += uint64(len(ul.Uids))
					}
				} else {
					algo.ApplyFilter(sg.uidMatrix[mIdx], func(uid uint64, i int) bool {
						key := fmt.Sprintf("%s|%d|%d", sg.Attr, fromUID, uid)
						_, seen := reachMap[key] // Combine fromUID here.
						if seen {
							return false
						}
						// Mark this edge as taken. We'd disallow this edge later.
						reachMap[key] = struct{}{}
						numEdges++
						return true
					})
				}
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
		var exp []*SubGraph
		for _, sg := range exec {
			if sg.UnknownAttr == true {
				continue
			}
			if len(sg.DestUIDs.Uids) == 0 {
				continue
			}
			if exp, err = expandChildren(ctx, sg, startChildren); err != nil {
				return err
			}
			out = append(out, exp...)
		}

		if numEdges > x.Config.QueryEdgeLimit {
			// If we've seen too many edges, stop the query.
			return errors.Errorf("Exceeded query edge limit = %v. Found %v edges.",
				x.Config.QueryEdgeLimit, numEdges)
		}

		if len(out) == 0 {
			return nil
		}
		exec = out
	}
}

// expandChildren adds child nodes to a SubGraph with no children, expanding them if necessary.
func expandChildren(ctx context.Context, sg *SubGraph, children []*SubGraph) ([]*SubGraph, error) {
	if len(sg.Children) > 0 {
		return nil, errors.New("Subgraph should not have any children")
	}
	// Add children and expand if necessary
	sg.Children = append(sg.Children, children...)
	expandedChildren, err := expandSubgraph(ctx, sg)
	if err != nil {
		return nil, err
	}
	out := make([]*SubGraph, 0, len(expandedChildren))
	sg.Children = sg.Children[:0]
	// Link new child nodes back to parent destination UIDs
	for _, child := range expandedChildren {
		newChild := new(SubGraph)
		newChild.copyFiltersRecurse(child)
		newChild.SrcUIDs = sg.DestUIDs
		newChild.Params.Var = child.Params.Var
		sg.Children = append(sg.Children, newChild)
		out = append(out, newChild)
	}
	return out, nil
}

func recurse(ctx context.Context, sg *SubGraph) error {
	if !sg.Params.Recurse {
		return errors.Errorf("Invalid recurse path query")
	}

	depth := sg.Params.RecurseArgs.Depth
	if depth == 0 {
		if sg.Params.RecurseArgs.AllowLoop {
			return errors.Errorf("Depth must be > 0 when loop is true for recurse query")
		}
		// If no depth is specified, expand till we reach all leaf nodes
		// or we see reach too many nodes.
		depth = math.MaxUint64
	}

	for _, child := range sg.Children {
		if len(child.Children) > 0 {
			return errors.Errorf(
				"recurse queries require that all predicates are specified in one level")
		}
	}

	return sg.expandRecurse(ctx, depth)
}
