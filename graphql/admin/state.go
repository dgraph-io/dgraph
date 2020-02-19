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
	Zeros      []member       `json:"zeros,omitempty"`
	MaxLeaseId uint64         `json:"maxLeaseId,omitempty"`
	MaxTxnTs   uint64         `json:"maxTxnTs,omitempty"`
	MaxRaftId  uint64         `json:"maxRaftId,omitempty"`
	Removed    []member       `json:"removed,omitempty"`
	Cid        string         `json:"cid,omitempty"`
	License    *pb.License    `json:"license,omitempty"`
}

type clusterGroup struct {
	Id         uint32   `json:"id,omitempty"`
	Members    []member `json:"members,omitempty"`
	Tablets    []tablet `json:"tablets,omitempty"`
	SnapshotTs uint64   `json:"snapshotTs,omitempty"`
	Checksum   uint64   `json:"checksum,omitempty"`
}

type member struct {
	Id              uint64 `json:"id,omitempty"`
	GroupId         uint32 `json:"groupId,omitempty"`
	Addr            string `json:"addr,omitempty"`
	Leader          bool   `json:"leader,omitempty"`
	AmDead          bool   `json:"amDead,omitempty"`
	LastUpdate      uint64 `json:"lastUpdate,omitempty"`
	ClusterInfoOnly bool   `json:"clusterInfoOnly,omitempty"`
	ForceGroupId    bool   `json:"forceGroupId,omitempty"`
}

type tablet struct {
	GroupId   uint32 `json:"groupId,omitempty"`
	Predicate string `json:"predicate,omitempty"`
	Force     bool   `json:"force,omitempty"`
	Space     int64  `json:"space,omitempty"`
	Remove    bool   `json:"remove,omitempty"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
	MoveTs    uint64 `json:"moveTs,omitempty"`
}

func (hr *stateResolver) Rewrite(q schema.Query) (*gql.GraphQuery, error) {
	return nil, nil
}

func (hr *stateResolver) Query(ctx context.Context, query *gql.GraphQuery) ([]byte, error) {
	var buf bytes.Buffer
	x.Check2(buf.WriteString(`{ "state":`))

	var resp *api.Response
	var respErr error
	if resp, respErr = (&edgraph.Server{}).State(ctx); respErr != nil {
		respErr = errors.Errorf("%s: %s", x.Error, respErr.Error())
		x.Check2(buf.Write([]byte(` null `)))
	} else {
		// unmarshal it back to MembershipState in order to map to graphql response
		u := jsonpb.Unmarshaler{}
		var ms pb.MembershipState
		_ = u.Unmarshal(bytes.NewReader(resp.GetJson()), &ms)

		// map to graphql response
		var graphQlMs membershipState
		graphQlMs.Counter = ms.Counter
		for k, v := range ms.Groups {
			var members []member
			for _, v1 := range v.Members {
				members = append(members, convertToMember(v1))
			}
			var tablets []tablet
			for _, v1 := range v.Tablets {
				tablets = append(tablets, convertToTablet(v1))
			}
			graphQlMs.Groups = append(graphQlMs.Groups, clusterGroup{
				Id:         k,
				Members:    members,
				Tablets:    tablets,
				SnapshotTs: v.SnapshotTs,
				Checksum:   v.Checksum,
			})
		}
		for _, v := range ms.Zeros {
			graphQlMs.Zeros = append(graphQlMs.Zeros, convertToMember(v))
		}
		graphQlMs.MaxLeaseId = ms.MaxLeaseId
		graphQlMs.MaxTxnTs = ms.MaxTxnTs
		graphQlMs.MaxRaftId = ms.MaxRaftId
		graphQlMs.Removed = convertToMemberSlice(ms.Removed)
		graphQlMs.Cid = ms.Cid
		graphQlMs.License = ms.License

		b, _ := json.Marshal(graphQlMs)
		x.Check2(buf.Write(b))
	}
	x.Check2(buf.WriteString(`}`))

	return buf.Bytes(), respErr
}

func convertToMemberSlice(pbMembers []*pb.Member) []member {
	members := make([]member, 0, len(pbMembers))
	for _, v := range pbMembers {
		members = append(members, convertToMember(v))
	}
	return members
}

func convertToMember(pbMember *pb.Member) member {
	return member{
		Id:              pbMember.Id,
		GroupId:         pbMember.GroupId,
		Addr:            pbMember.Addr,
		Leader:          pbMember.Leader,
		AmDead:          pbMember.AmDead,
		LastUpdate:      pbMember.LastUpdate,
		ClusterInfoOnly: pbMember.ClusterInfoOnly,
		ForceGroupId:    pbMember.ForceGroupId,
	}
}

func convertToTabletSlice(pbTablets []*pb.Tablet) []tablet {
	tablets := make([]tablet, 0, len(pbTablets))
	for _, v := range pbTablets {
		tablets = append(tablets, convertToTablet(v))
	}
	return tablets
}

func convertToTablet(pbTablet *pb.Tablet) tablet {
	return tablet{
		GroupId:   pbTablet.GroupId,
		Predicate: pbTablet.Predicate,
		Force:     pbTablet.Force,
		Space:     pbTablet.Space,
		Remove:    pbTablet.Remove,
		ReadOnly:  pbTablet.ReadOnly,
		MoveTs:    pbTablet.MoveTs,
	}
}
