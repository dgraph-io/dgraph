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

func AssignUidsForDgraphLite(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
	uidCounter := groups().state.MaxUID
	if uidCounter == 0 {
		uidCounter = 1
	}
	if num.Bump {
		if uidCounter >= num.Val {
			panic("invalid request")
		}
		groups().state.MaxUID = num.Val
		return &pb.AssignedIds{StartId: uidCounter + 1, EndId: num.Val + 1}, nil
	}

	groups().state.MaxUID += num.Val
	return &pb.AssignedIds{StartId: uidCounter + 1, EndId: uidCounter + num.Val + 1}, nil
}
