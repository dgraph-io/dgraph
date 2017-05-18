package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

type MutationResult struct {
	Edges   []*protos.DirectedEdge
	NewUids map[string]uint64
	EdgeOps []protos.DirectedEdge_Op
}

func ApplyMutations(ctx context.Context, m *protos.Mutations) error {
	err := AddInternalEdge(ctx, m)
	if err != nil {
		return x.Wrapf(err, "While adding internal edges")
	}
	err = worker.MutateOverNetwork(ctx, m)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while MutateOverNetwork"))
		return err
	}
	return nil
}

func AddInternalEdge(ctx context.Context, m *protos.Mutations) error {
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
			if mu.Attr != x.DeleteAllPredicates {
				newEdges = append(newEdges, mu)
				if string(mu.GetValue()) == x.DeleteAllObjects {
					// Delete the given predicate from _predicate_.
					edge := &protos.DirectedEdge{
						Op:     protos.DirectedEdge_DEL,
						Entity: mu.GetEntity(),
						Attr:   "_predicate_",
						Value:  []byte(mu.GetAttr()),
					}
					newEdges = append(newEdges, edge)
				}
			} else {
				// Fetch all the predicates and replace them
				preds, err := GetNodePredicates(ctx, &protos.List{[]uint64{mu.GetEntity()}})
				if err != nil {
					return err
				}
				val := mu.GetValue()
				for _, pred := range preds {
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
			}
		}
	}
	m.Edges = newEdges
	return nil
}

func ConvertToEdges(ctx context.Context,
	nquads gql.NQuads,
	vars map[string]varValue) (MutationResult, error) {
	var mr MutationResult

	newUids := make(map[string]uint64)
	for _, nq := range nquads.NQuads {
		if len(nq.Subject) > 0 {
			if strings.HasPrefix(nq.Subject, "_:") {
				newUids[nq.Subject] = 0
			} else {
				// Only store xids that need to be marked as used.
				_, err := gql.ParseUid(nq.Subject)
				if err == gql.ErrInvalidUID {
					return mr, err
				} else if err != nil {
					newUids[nq.Subject] = 0
				}
			}
		}

		if len(nq.ObjectId) > 0 {
			if strings.HasPrefix(nq.ObjectId, "_:") {
				newUids[nq.ObjectId] = 0
			} else {
				_, err := gql.ParseUid(nq.ObjectId)
				if err == gql.ErrInvalidUID {
					return mr, err
				} else if err != nil {
					newUids[nq.ObjectId] = 0
				}
			}
		}

	}

	if len(newUids) > 0 {
		if err := worker.AssignUidsOverNetwork(ctx, newUids); err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while AssignUidsOverNetwork for newUids: %v", newUids))
			return mr, err
		}
	}

	// Wrapper for a pointer to protos.Nquad
	var wnq gql.NQuad
	for i, nq := range nquads.NQuads {
		wnq = gql.NQuad{nq}
		fmt.Println(nq.String())
		if len(nq.Subject) > 0 {
			// Get edges from nquad using newUids.
			edge, err := wnq.ToEdgeUsing(newUids)
			if err != nil {
				x.TraceError(ctx, x.Wrapf(err, "Error while converting to edge: %v", nq))
				return mr, err
			}
			mr.Edges = append(mr.Edges, edge)
			mr.EdgeOps = append(mr.EdgeOps, nquads.Types[i])
		} else { // variable in subject
			x.AssertTrue(len(nq.SubjectVar) > 0)
			expanded, err := wnq.ExpandSubjectVar(vars[nq.SubjectVar].Uids.Uids, newUids)
			if err != nil {
				return mr, err
			}
			mr.Edges = append(mr.Edges, expanded...)
			for _ = range expanded {
				fmt.Println(expanded[i].String())
				mr.EdgeOps = append(mr.EdgeOps, nquads.Types[i])
			}
		}
	}

	mr.NewUids = make(map[string]uint64)
	// Strip out _: prefix from the blank node keys.
	for k, v := range newUids {
		if strings.HasPrefix(k, "_:") {
			mr.NewUids[k[2:]] = v
		}
	}

	return mr, nil
}

// ConvertAndApply materializes edges defined by the mutation
// and adds them to the database.
func ConvertAndApply(ctx context.Context, mutation *protos.Mutation) (map[string]uint64, error) {
	var allocIds map[string]uint64
	var m protos.Mutations
	var err error
	var mr MutationResult

	nquads := gql.WrapNQ(mutation.Set, protos.DirectedEdge_SET)
	if mr, err = ConvertToEdges(ctx, nquads, nil); err != nil {
		return nil, err
	}
	m.Edges, allocIds = mr.Edges, mr.NewUids
	for i := range m.Edges {
		m.Edges[i].Op = mr.EdgeOps[i]
	}

	nquads = gql.WrapNQ(mutation.Del, protos.DirectedEdge_DEL)
	if mr, err = ConvertToEdges(ctx, nquads, nil); err != nil {
		return nil, err
	}
	for i := range mr.Edges {
		edge := mr.Edges[i]
		edge.Op = mr.EdgeOps[i]
		m.Edges = append(m.Edges, edge)
	}

	m.Schema = mutation.Schema
	if err := ApplyMutations(ctx, &m); err != nil {
		return nil, x.Wrap(err)
	}
	return allocIds, nil
}
