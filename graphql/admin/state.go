package admin

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/dgraph-io/dgraph/v24/edgraph"
	"github.com/dgraph-io/dgraph/v24/graphql/resolve"
	"github.com/dgraph-io/dgraph/v24/graphql/schema"
	"github.com/dgraph-io/dgraph/v24/protos/pb"
	"github.com/dgraph-io/dgraph/v24/x"
)

type membershipState struct {
	Counter    uint64         `json:"counter"`
	Groups     []clusterGroup `json:"groups"`
	Zeros      []member       `json:"zeros"`
	MaxUID     uint64         `json:"maxUID"`
	MaxNsID    uint64         `json:"maxNsID"`
	MaxTxnTs   uint64         `json:"maxTxnTs"`
	MaxRaftId  uint64         `json:"maxRaftId"`
	Removed    []*pb.Member   `json:"removed"`
	Cid        string         `json:"cid"`
	License    *pb.License    `json:"license"`
	Namespaces []uint64       `json:"namespaces"`
}

type clusterGroup struct {
	Id         uint32   `json:"id"`
	Members    []member `json:"members"`
	Tablets    []tablet `json:"tablets"`
	SnapshotTs uint64   `json:"snapshotTs"`
	Checksum   uint64   `json:"checksum"`
}

type member struct {
	Id              uint64 `json:"id"`
	GroupId         uint32 `json:"groupId"`
	Addr            string `json:"addr"`
	Leader          bool   `json:"leader"`
	AmDead          bool   `json:"amDead"`
	LastUpdate      uint64 `json:"lastUpdate"`
	ClusterInfoOnly bool   `json:"clusterInfoOnly"`
	Learner         bool   `json:"learner"`
	ForceGroupId    bool   `json:"forceGroupId"`
}

type tablet struct {
	GroupId           uint32 `json:"groupId"`
	Predicate         string `json:"predicate"`
	Force             bool   `json:"force"`
	OnDiskBytes       int64  `json:"onDiskBytes"`
	Remove            bool   `json:"remove"`
	ReadOnly          bool   `json:"readOnly"`
	MoveTs            uint64 `json:"moveTs"`
	UncompressedBytes int64  `json:"uncompressedBytes"`
}

func resolveState(ctx context.Context, q schema.Query) *resolve.Resolved {
	resp, err := (&edgraph.Server{}).State(ctx)
	if err != nil {
		return resolve.EmptyResult(q, errors.Errorf("%s: %s", x.Error, err.Error()))
	}

	// unmarshal it back to MembershipState proto in order to map to graphql response
	var ms pb.MembershipState
	err = protojson.Unmarshal(resp.GetJson(), &ms)
	if err != nil {
		return resolve.EmptyResult(q, err)
	}

	ns, _ := x.ExtractNamespace(ctx)
	// map to graphql response structure. Only guardian of galaxy can list the namespaces.
	state := convertToGraphQLResp(&ms, ns == x.GalaxyNamespace)
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
		var members = make([]member, 0, len(v.Members))
		for id, v1 := range v.Members {
			members = append(members, member{
				Id:              id,
				GroupId:         v1.GroupId,
				Addr:            v1.Addr,
				Leader:          v1.Leader,
				AmDead:          v1.AmDead,
				LastUpdate:      v1.LastUpdate,
				ClusterInfoOnly: v1.ClusterInfoOnly,
				ForceGroupId:    v1.ForceGroupId,
			})
		}
		var tablets = make([]tablet, 0, len(v.Tablets))
		for name, v1 := range v.Tablets {
			tablets = append(tablets, tablet{
				GroupId:           v1.GroupId,
				Predicate:         v1.Predicate,
				Force:             v1.Force,
				OnDiskBytes:       v1.OnDiskBytes,
				Remove:            v1.Remove,
				ReadOnly:          v1.ReadOnly,
				MoveTs:            v1.MoveTs,
				UncompressedBytes: v1.UncompressedBytes,
			})
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
	state.Zeros = make([]member, 0, len(ms.Zeros))
	for _, v := range ms.Zeros {
		state.Zeros = append(state.Zeros, member{
			Id:              v.Id,
			GroupId:         v.GroupId,
			Addr:            v.Addr,
			Leader:          v.Leader,
			AmDead:          v.AmDead,
			LastUpdate:      v.LastUpdate,
			ClusterInfoOnly: v.ClusterInfoOnly,
			ForceGroupId:    v.ForceGroupId,
		})
	}
	state.MaxUID = ms.MaxUID
	state.MaxTxnTs = ms.MaxTxnTs
	state.MaxNsID = ms.MaxNsID
	state.MaxRaftId = ms.MaxRaftId
	state.Removed = ms.Removed
	state.Cid = ms.Cid
	state.License = ms.License

	state.Namespaces = []uint64{}
	for ns := range namespaces {
		state.Namespaces = append(state.Namespaces, ns)
	}

	return state
}
