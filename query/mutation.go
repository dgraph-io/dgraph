package query

import (
	"context"
	"strings"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

type InternalMutation struct {
	Edges []*protos.DirectedEdge
}

func (mr *InternalMutation) AddEdge(edge *protos.DirectedEdge, op protos.DirectedEdge_Op) {
	edge.Op = op
	mr.Edges = append(mr.Edges, edge)
}

func ApplyMutations(ctx context.Context, m *protos.Mutations) error {
	if worker.Config.ExpandEdge {
		err := addInternalEdge(ctx, m)
		if err != nil {
			return x.Wrapf(err, "While adding internal edges")
		}
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Added Internal edges")
		}
	}
	if err := worker.MutateOverNetwork(ctx, m); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while MutateOverNetwork: %+v", err)
		}
		return err
	}
	return nil
}

func addInternalEdge(ctx context.Context, m *protos.Mutations) error {
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
				preds, err := GetNodePredicates(ctx, &protos.List{[]uint64{mu.GetEntity()}})
				if err != nil {
					return err
				}
				val := mu.GetValue()
				for _, pred := range preds {
					edge := &protos.DirectedEdge{
						Op:     protos.DirectedEdge_DEL,
						Entity: mu.GetEntity(),
						Attr:   string(pred.Values[0].Val),
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

func AssignUids(ctx context.Context, nquads gql.NQuads) (map[string]uint64, error) {
	newUids := make(map[string]uint64)
	num := &protos.Num{}
	var err error
	for _, nq := range nquads.NQuads {
		// We dont want to assign uids to these.
		if nq.Subject == x.Star && nq.ObjectValue.GetDefaultVal() == x.Star {
			continue
		}

		if len(nq.Subject) > 0 {
			if strings.HasPrefix(nq.Subject, "_:") {
				newUids[nq.Subject] = 0
				num.Val = num.Val + 1
			} else if _, err := gql.ParseUid(nq.Subject); err != nil {
				return newUids, err
			}
		}

		if len(nq.ObjectId) > 0 {
			if strings.HasPrefix(nq.ObjectId, "_:") {
				newUids[nq.ObjectId] = 0
				num.Val = num.Val + 1
			} else if _, err := gql.ParseUid(nq.ObjectId); err != nil {
				return newUids, err
			}
		}
	}

	if int(num.Val) > 0 {
		var res *protos.AssignedIds
		// TODO: Optimize later by prefetching
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

func expandVariables(nq *gql.NQuad,
	newUids map[string]uint64,
	vars map[string]varValue) ([]*protos.DirectedEdge, error) {
	var subjectUids, objectUids []uint64
	if len(nq.SubjectVar) > 0 {
		if vars[nq.SubjectVar].Uids != nil {
			subjectUids = vars[nq.SubjectVar].Uids.Uids
		}
	}
	if len(nq.ObjectVar) > 0 {
		if vars[nq.ObjectVar].Uids != nil {
			objectUids = vars[nq.ObjectVar].Uids.Uids
		}
	}
	return nq.ExpandVariables(newUids, subjectUids, objectUids)
}
func ToInternal(ctx context.Context,
	nquads gql.NQuads,
	vars map[string]varValue, newUids map[string]uint64) (InternalMutation, error) {
	var mr InternalMutation
	var err error

	if newUids == nil {
		newUids = make(map[string]uint64)
	}

	// Wrapper for a pointer to protos.Nquad
	var wnq *gql.NQuad
	var delPred []*protos.NQuad
	for i, nq := range nquads.NQuads {
		if nq.Subject == x.Star && nq.ObjectValue.GetDefaultVal() == x.Star {
			delPred = append(delPred, nq)
			continue
		}

		wnq = &gql.NQuad{nq}
		usesVariable := len(nq.SubjectVar) > 0 || len(nq.ObjectVar) > 0

		if !usesVariable {
			if len(nq.Subject) > 0 {
				// Get edge from nquad using newUids.
				var edge *protos.DirectedEdge
				edge, err = wnq.ToEdgeUsing(newUids)
				if err != nil {
					return mr, x.Wrap(err)
				}
				mr.AddEdge(edge, nquads.Types[i])
			}
		} else {
			var expanded []*protos.DirectedEdge
			expanded, err = expandVariables(wnq, newUids, vars)
			if err != nil {
				return mr, x.Wrap(err)
			}
			for _, edge := range expanded {
				mr.AddEdge(edge, nquads.Types[i])
			}
		}
	}

	for i, nq := range delPred {
		wnq = &gql.NQuad{nq}
		edge, err := wnq.ToDeletePredEdge()
		if err != nil {
			return mr, err
		}
		mr.AddEdge(edge, nquads.Types[i])
	}

	return mr, nil
}
