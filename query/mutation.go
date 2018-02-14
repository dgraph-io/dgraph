package query

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos/api"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

func ApplyMutations(ctx context.Context, m *intern.Mutations) (*api.TxnContext, error) {
	if worker.Config.ExpandEdge {
		edges, err := expandEdges(ctx, m)
		if err != nil {
			return nil, x.Wrapf(err, "While adding intern.edges")
		}
		m.Edges = edges
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

func expandEdges(ctx context.Context, m *intern.Mutations) ([]*intern.DirectedEdge, error) {
	edges := make([]*intern.DirectedEdge, 0, 2*len(m.Edges))
	for _, edge := range m.Edges {
		x.AssertTrue(edge.Op == intern.DirectedEdge_DEL || edge.Op == intern.DirectedEdge_SET)

		var preds []string
		if edge.Attr != x.Star {
			preds = []string{edge.Attr}
		} else {
			sg := &SubGraph{}
			sg.DestUIDs = &intern.List{[]uint64{edge.GetEntity()}}
			sg.ReadTs = m.StartTs
			valMatrix, err := getNodePredicates(ctx, sg)
			if err != nil {
				return nil, err
			}
			if len(valMatrix) != 1 {
				return nil, x.Errorf("Expected only one list in value matrix while deleting: %v",
					edge.GetEntity())
			}
			for _, tv := range valMatrix[0].Values {
				if len(tv.Val) > 0 {
					preds = append(preds, string(tv.Val))
				}
			}
		}

		for _, pred := range preds {
			edgeCopy := *edge
			edgeCopy.Attr = pred
			edges = append(edges, &edgeCopy)

			e := &intern.DirectedEdge{
				Op:     edge.Op,
				Entity: edge.GetEntity(),
				Attr:   "_predicate_",
				Value:  []byte(pred),
			}
			edges = append(edges, e)
		}
	}
	return edges, nil
}

func verifyUid(uid uint64) error {
	if uid <= worker.MaxLeaseId() {
		return nil
	}
	// Even though the uid is above the max lease id, it might just be because
	// the membership state has fallen behind. Update the state and try again.
	if err := worker.UpdateMembershipState(ctx); err != nil {
		return x.Wrapf(err, "updating error state")
	}
	if lease := worker.MaxLeaseId(); uid > lease {
		return fmt.Errorf("Uid: [%d] cannot be greater than lease: [%d]", uid, lease)
	}
	return nil
}

func AssignUids(ctx context.Context, nquads []*api.NQuad) (map[string]uint64, error) {
	newUids := make(map[string]uint64)
	num := &intern.Num{}
	var err error
	for _, nq := range nquads {
		// We dont want to assign uids to these.
		if nq.Subject == x.Star && nq.ObjectValue.GetDefaultVal() == x.Star {
			return newUids, errors.New("Predicate deletion should be called via alter.")
		}

		if len(nq.Subject) == 0 {
			return nil, fmt.Errorf("Subject must not be empty for nquad: %+v", nq)
		}
		var uid uint64
		if strings.HasPrefix(nq.Subject, "_:") {
			newUids[nq.Subject] = 0
		} else if uid, err = gql.ParseUid(nq.Subject); err != nil {
			return newUids, err
		}
		if err = verifyUid(uid); err != nil {
			return newUids, err
		}

		if len(nq.ObjectId) > 0 {
			var uid uint64
			if strings.HasPrefix(nq.ObjectId, "_:") {
				newUids[nq.ObjectId] = 0
			} else if uid, err = gql.ParseUid(nq.ObjectId); err != nil {
				return newUids, err
			}
			if err = verifyUid(uid); err != nil {
				return newUids, err
			}
		}
	}

	num.Val = uint64(len(newUids))
	if int(num.Val) > 0 {
		var res *api.AssignedIds
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
	newUids map[string]uint64) (edges []*intern.DirectedEdge, err error) {

	// Wrapper for a pointer to protos.Nquad
	var wnq *gql.NQuad

	parse := func(nq *api.NQuad, op intern.DirectedEdge_Op) error {
		wnq = &gql.NQuad{nq}
		if len(nq.Subject) == 0 {
			return nil
		}
		// Get edge from nquad using newUids.
		var edge *intern.DirectedEdge
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
		if err := parse(nq, intern.DirectedEdge_SET); err != nil {
			return edges, err
		}
	}
	for _, nq := range gmu.Del {
		if nq.Subject == x.Star && nq.ObjectValue.GetDefaultVal() == x.Star {
			return edges, errors.New("Predicate deletion should be called via alter.")
		}
		if err := parse(nq, intern.DirectedEdge_DEL); err != nil {
			return edges, err
		}
	}

	return edges, nil
}
