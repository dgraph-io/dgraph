package worker

import (
	"context"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/dgraph/v24/conn"
	"github.com/dgraph-io/dgraph/v24/protos/pb"
	"github.com/dgraph-io/dgraph/v24/schema"
)

func InitForLite(ps *badger.DB) {
	pstore = ps
	groups().state = &pb.MembershipState{}
	groups().Node = &node{Node: &conn.Node{Id: 1}}
	groups().gid = 1
}

func InitTablet(pred string) {
	groups().Lock()
	defer groups().Unlock()
	groups().tablets[pred] = &pb.Tablet{GroupId: 1, Predicate: pred}
}

func ApplyMutations(ctx context.Context, p *pb.Proposal) error {
	return groups().Node.applyMutations(ctx, p)
}

func ApplyCommited(ctx context.Context, delta *pb.OracleDelta) error {
	return groups().Node.commitOrAbort(1, delta)
}

func ApplyInitialSchema() error {
	for _, su := range schema.InitialSchema(0) {
		if err := updateSchema(su, 1); err != nil {
			return err
		}

		func() {
			groups().Lock()
			defer groups().Unlock()
			groups().tablets[su.Predicate] = &pb.Tablet{GroupId: groups().groupId(), Predicate: su.Predicate}
		}()
	}
	gr.applyInitialTypes()
	return nil
}

func SetMaxUID(uid uint64) {
	groups().Lock()
	defer groups().Unlock()
	groups().state.MaxUID = uid
}
