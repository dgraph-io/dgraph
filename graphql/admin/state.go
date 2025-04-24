/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package admin

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/hypermodeinc/dgraph/v25/edgraph"
	"github.com/hypermodeinc/dgraph/v25/graphql/resolve"
	"github.com/hypermodeinc/dgraph/v25/graphql/schema"
	"github.com/hypermodeinc/dgraph/v25/protos/pb"
	"github.com/hypermodeinc/dgraph/v25/x"
)

type membershipState struct {
	Counter    uint64         `json:"counter,omitempty"`
	Groups     []clusterGroup `json:"groups,omitempty"`
	Zeros      []*pb.Member   `json:"zeros,omitempty"`
	MaxUID     uint64         `json:"maxUID,omitempty"`
	MaxNsID    uint64         `json:"maxNsID,omitempty"`
	MaxTxnTs   uint64         `json:"maxTxnTs,omitempty"`
	MaxRaftId  uint64         `json:"maxRaftId,omitempty"`
	Removed    []*pb.Member   `json:"removed,omitempty"`
	Cid        string         `json:"cid,omitempty"`
	Namespaces []uint64       `json:"namespaces,omitempty"`
}

type clusterGroup struct {
	Id         uint32       `json:"id,omitempty"`
	Members    []*pb.Member `json:"members,omitempty"`
	Tablets    []*pb.Tablet `json:"tablets,omitempty"`
	SnapshotTs uint64       `json:"snapshotTs,omitempty"`
	Checksum   uint64       `json:"checksum,omitempty"`
}

func resolveState(ctx context.Context, q schema.Query) *resolve.Resolved {
	resp, err := (&edgraph.Server{}).State(ctx)
	if err != nil {
		return resolve.EmptyResult(q, errors.Errorf("%s: %s", x.Error, err.Error()))
	}

	// unmarshal it back to MembershipState proto in order to map to graphql response
	var ms pb.MembershipState
	if err := protojson.Unmarshal(resp.GetJson(), &ms); err != nil {
		return resolve.EmptyResult(q, err)
	}

	ns, _ := x.ExtractNamespace(ctx)
	// map to graphql response structure. Only superadmin can list the namespaces.
	state := convertToGraphQLResp(&ms, ns == x.RootNamespace)
	b, err := json.Marshal(state)
	if err != nil {
		return resolve.EmptyResult(q, err)
	}
	var resultState map[string]interface{}
	err = schema.Unmarshal(b, &resultState)
	if err != nil {
		return resolve.EmptyResult(q, err)
	}

	return resolve.DataResult(
		q,
		map[string]interface{}{q.Name(): resultState},
		nil,
	)
}

// convertToGraphQLResp converts MembershipState proto to GraphQL layer response
// MembershipState proto contains some fields which are of type map, and as GraphQL
// does not have a map type, we convert those maps to lists by using just the map
// values and not the keys. For pb.MembershipState.Group, the keys are the group IDs
// and pb.Group didn't contain this ID, so we are creating a custom clusterGroup type,
// which is same as pb.Group and also contains the ID for the group.
func convertToGraphQLResp(ms *pb.MembershipState, listNs bool) membershipState {
	var state membershipState

	// namespaces stores set of namespaces
	namespaces := make(map[uint64]struct{})

	state.Counter = ms.Counter
	for k, v := range ms.Groups {
		var members = make([]*pb.Member, 0, len(v.Members))
		for _, v1 := range v.Members {
			members = append(members, v1)
		}
		var tablets = make([]*pb.Tablet, 0, len(v.Tablets))
		for name, v1 := range v.Tablets {
			tablets = append(tablets, v1)
			if listNs {
				namespaces[x.ParseNamespace(name)] = struct{}{}
			}
		}
		state.Groups = append(state.Groups, clusterGroup{
			Id:         k,
			Members:    members,
			Tablets:    tablets,
			SnapshotTs: v.SnapshotTs,
			Checksum:   v.Checksum,
		})
	}
	state.Zeros = make([]*pb.Member, 0, len(ms.Zeros))
	for _, v := range ms.Zeros {
		state.Zeros = append(state.Zeros, v)
	}
	state.MaxUID = ms.MaxUID
	state.MaxTxnTs = ms.MaxTxnTs
	state.MaxNsID = ms.MaxNsID
	state.MaxRaftId = ms.MaxRaftId
	state.Removed = ms.Removed
	state.Cid = ms.Cid

	state.Namespaces = []uint64{}
	for ns := range namespaces {
		state.Namespaces = append(state.Namespaces, ns)
	}

	return state
}
