package query

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

func ApplyMutations(ctx context.Context, m *protos.Mutations) (*protos.TxnContext, error) {
	if worker.Config.ExpandEdge {
		err := handleInternalEdge(ctx, m)
		if err != nil {
			return nil, x.Wrapf(err, "While adding internal edges")
		}
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Added Internal edges")
		}
	} else {
		for _, mu := range m.Edges {
			if mu.Attr == x.Star && !worker.Config.ExpandEdge {
				return nil, x.Errorf("Expand edge (--expand_edge) is set to false." +
					" Cannot perform S * * deletion.")
			}
		}
	}
	tctx, err := worker.MutateOverNetwork(ctx, m)
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while MutateOverNetwork: %+v", err)
		}
	}
	return tctx, err
}

func handleInternalEdge(ctx context.Context, m *protos.Mutations) error {
	newEdges := make([]*protos.DirectedEdge, 0, 2*len(m.Edges))
	for _, mu := range m.Edges {
		x.AssertTrue(mu.Op == protos.DirectedEdge_DEL || mu.Op == protos.DirectedEdge_SET)
		if mu.Op == protos.DirectedEdge_SET {
			edge := &protos.DirectedEdge{
				Op:     protos.DirectedEdge_SET,
				Entity: mu.GetEntity(),
				Attr:   "_predicate_",
				Value:  []byte(mu.GetAttr()),
			}
			newEdges = append(newEdges, mu)
			newEdges = append(newEdges, edge)
		} else if mu.Op == protos.DirectedEdge_DEL {
			// S * * case
			if mu.Attr == x.Star {
				// Fetch all the predicates and replace them

				sg := &SubGraph{}
				sg.DestUIDs = &protos.List{[]uint64{mu.GetEntity()}}
				sg.ReadTs = m.StartTs
				valMatrix, err := getNodePredicates(ctx, sg)
				if err != nil {
					return err
				}

				// _predicate_ is of list type. So we will get all the predicates in the first list
				// of the value matrix.
				val := mu.GetValue()
				if len(valMatrix) != 1 {
					return x.Errorf("Expected only one list in value matrix while deleting: %v",
						mu.GetEntity())
				}
				preds := valMatrix[0].Values
				for _, pred := range preds {
					if bytes.Equal(pred.Val, x.Nilbyte) {
						continue
					}
					edge := &protos.DirectedEdge{
						Op:     protos.DirectedEdge_DEL,
						Entity: mu.GetEntity(),
						Attr:   string(pred.Val),
						Value:  val,
					}
					newEdges = append(newEdges, edge)
				}
				edge := &protos.DirectedEdge{
					Op:     protos.DirectedEdge_DEL,
					Entity: mu.GetEntity(),
					Attr:   "_predicate_",
					Value:  val,
				}
				// Delete all the _predicate_ values
				edge.Attr = "_predicate_"
				newEdges = append(newEdges, edge)

			} else {
				newEdges = append(newEdges, mu)
				if mu.Entity == 0 && string(mu.GetValue()) == x.Star {
					// * P * case.
					continue
				}

				// S P * case.
				if string(mu.GetValue()) == x.Star {
					// Delete the given predicate from _predicate_.
					edge := &protos.DirectedEdge{
						Op:     protos.DirectedEdge_DEL,
						Entity: mu.GetEntity(),
						Attr:   "_predicate_",
						Value:  []byte(mu.GetAttr()),
					}
					newEdges = append(newEdges, edge)
				}
			}
		}
	}
	m.Edges = newEdges
	return nil
}

func AssignUids(ctx context.Context, nquads []*protos.NQuad) (map[string]uint64, error) {
	newUids := make(map[string]uint64)
	num := &protos.Num{}
	var err error
	maxLeaseId := worker.MaxLeaseId()
	for _, nq := range nquads {
		// We dont want to assign uids to these.
		if nq.Subject == x.Star && nq.ObjectValue.GetDefaultVal() == x.Star {
			continue
		}

		if len(nq.Subject) > 0 {
			var uid uint64
			if strings.HasPrefix(nq.Subject, "_:") {
				newUids[nq.Subject] = 0
			} else if uid, err = gql.ParseUid(nq.Subject); err != nil {
				return newUids, err
			}
			if uid > maxLeaseId {
				return newUids, fmt.Errorf("Uid: [%d] cannot be greater than lease: [%d]", uid,
					maxLeaseId)
			}
		}

		if len(nq.ObjectId) > 0 {
			var uid uint64
			if strings.HasPrefix(nq.ObjectId, "_:") {
				newUids[nq.ObjectId] = 0
			} else if uid, err = gql.ParseUid(nq.ObjectId); err != nil {
				return newUids, err
			}
			if uid > maxLeaseId {
				return newUids, fmt.Errorf("Uid: [%d] cannot be greater than lease: [%d]", uid,
					maxLeaseId)
			}
		}
	}

	num.Val = uint64(len(newUids))
	if int(num.Val) > 0 {
		var res *protos.AssignedIds
		// TODO: Optimize later by prefetching. Also consolidate all the UID requests into a single
		// pending request from this server to zero.
		if res, err = worker.AssignUidsOverNetwork(ctx, num); err != nil {
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Error while AssignUidsOverNetwork for newUids: %+v", err)
			}
			return newUids, err
		}
		curId := res.StartId
		// assign generated ones now
		for k := range newUids {
			x.AssertTruef(curId != 0 && curId <= res.EndId, "not enough uids generated")
			newUids[k] = curId
			curId++
		}
	}
	return newUids, nil
}

func ToInternal(gmu *gql.Mutation,
	newUids map[string]uint64) (edges []*protos.DirectedEdge, err error) {

	// Wrapper for a pointer to protos.Nquad
	var wnq *gql.NQuad

	parse := func(nq *protos.NQuad, op protos.DirectedEdge_Op) error {
		wnq = &gql.NQuad{nq}
		if len(nq.Subject) == 0 {
			return nil
		}
		// Get edge from nquad using newUids.
		var edge *protos.DirectedEdge
		edge, err = wnq.ToEdgeUsing(newUids)
		if err != nil {
			return x.Wrap(err)
		}
		edge.Op = op
		edges = append(edges, edge)
		return nil
	}

	for _, nq := range gmu.Set {
		if err := facets.SortAndValidate(nq.Facets); err != nil {
			return edges, err
		}
		if err := parse(nq, protos.DirectedEdge_SET); err != nil {
			return edges, err
		}
	}
	for _, nq := range gmu.Del {
		if nq.Subject == x.Star && nq.ObjectValue.GetDefaultVal() == x.Star {
			return edges, errors.New("Predicate deletion should be called via alter.")
		}
		if err := parse(nq, protos.DirectedEdge_DEL); err != nil {
			return edges, err
		}
	}

	return edges, nil
}
