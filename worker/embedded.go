/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"context"

	"github.com/golang/glog"

	"github.com/dgraph-io/badger/v4"
	"github.com/hypermodeinc/dgraph/v25/conn"
	"github.com/hypermodeinc/dgraph/v25/protos/pb"
	"github.com/hypermodeinc/dgraph/v25/schema"
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

func ApplyInitialSchema(ns, ts uint64) error {
	for _, su := range schema.InitialSchema(ns) {
		if err := updateSchema(su, ts); err != nil {
			return err
		}
	}
	applyInitialTypes(ns, ts)
	return nil
}

func applyInitialTypes(ns, ts uint64) {
	initialTypes := schema.InitialTypes(ns)
	for _, t := range initialTypes {
		if _, ok := schema.State().GetType(t.TypeName); ok {
			continue
		}
		// It is okay to write initial types at ts=1.
		if err := updateType(t.GetTypeName(), t, ts); err != nil {
			glog.Errorf("Error while applying initial type: %s", err)
		}
	}
}

func SetMaxUID(uid uint64) {
	groups().Lock()
	defer groups().Unlock()
	groups().state.MaxUID = uid
}

func SetMaxNsID(nsId uint64) {
	groups().Lock()
	defer groups().Unlock()
	groups().state.MaxNsID = nsId
}
