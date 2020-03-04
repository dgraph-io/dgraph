package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/pkg/errors"
)

type stateResolver struct {
}

type membershipState struct {
	Counter    uint64         `json:"counter,omitempty"`
	Groups     []clusterGroup `json:"groups,omitempty"`
	Zeros      []*pb.Member   `json:"zeros,omitempty"`
	MaxLeaseId uint64         `json:"maxLeaseId,omitempty"`
	MaxTxnTs   uint64         `json:"maxTxnTs,omitempty"`
	MaxRaftId  uint64         `json:"maxRaftId,omitempty"`
	Removed    []*pb.Member   `json:"removed,omitempty"`
	Cid        string         `json:"cid,omitempty"`
	License    *pb.License    `json:"license,omitempty"`
}

type clusterGroup struct {
	Id         uint32       `json:"id,omitempty"`
	Members    []*pb.Member `json:"members,omitempty"`
	Tablets    []*pb.Tablet `json:"tablets,omitempty"`
	SnapshotTs uint64       `json:"snapshotTs,omitempty"`
	Checksum   uint64       `json:"checksum,omitempty"`
}

func (hr *stateResolver) Rewrite(ctx context.Context, q schema.Query) (*gql.GraphQuery, error) {
	return nil, nil
}

func (hr *stateResolver) Query(ctx context.Context, query *gql.GraphQuery) ([]byte, error) {
	var buf bytes.Buffer
	var resp *api.Response
	var err error

	x.Check2(buf.WriteString(`{ "state":`))

	if resp, err = (&edgraph.Server{}).State(ctx); err != nil {
		err = errors.Errorf("%s: %s", x.Error, err.Error())
		x.Check2(buf.Write([]byte(` null `)))
	} else {
		// unmarshal it back to MembershipState proto in order to map to graphql response
		u := jsonpb.Unmarshaler{}
		var ms pb.MembershipState
		err = u.Unmarshal(bytes.NewReader(resp.GetJson()), &ms)

		if err != nil {
			x.Check2(buf.Write([]byte(` null `)))
		} else {
			// map to graphql response
			state := convertToGraphQLResp(ms)

			// marshal it back to json
			var b []byte
			b, err = json.Marshal(state)
			if err != nil {
				x.Check2(buf.Write([]byte(` null `)))
			} else {
				x.Check2(buf.Write(b))
			}
		}
	}
	x.Check2(buf.WriteString(`}`))

	return buf.Bytes(), err
}

// convertToGraphQLResp converts MembershipState proto to GraphQL layer response
// MembershipState proto contains some fields which are of type map, and as GraphQL
// does not have a map type, we convert those maps to lists by using just the map
// values and not the keys. For pb.MembershipState.Group, the keys are the group IDs
// and pb.Group didn't contain this ID, so we are creating a custom clusterGroup type,
// which is same as pb.Group and also contains the ID for the group.
func convertToGraphQLResp(ms pb.MembershipState) membershipState {
	var state membershipState

	state.Counter = ms.Counter
	for k, v := range ms.Groups {
		var members = make([]*pb.Member, 0, len(v.Members))
		for _, v1 := range v.Members {
			members = append(members, v1)
		}
		var tablets = make([]*pb.Tablet, 0, len(v.Tablets))
		for _, v1 := range v.Tablets {
			tablets = append(tablets, v1)
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
	state.MaxLeaseId = ms.MaxLeaseId
	state.MaxTxnTs = ms.MaxTxnTs
	state.MaxRaftId = ms.MaxRaftId
	state.Removed = ms.Removed
	state.Cid = ms.Cid
	state.License = ms.License

	return state
}
