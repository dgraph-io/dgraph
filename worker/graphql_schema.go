/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package worker

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/pkg/errors"
)

var (
	schemaLock                            sync.Mutex
	errUpdatingGQLSchemaOnNonGroup1Leader = errors.New(
		"while updating GraphQL schema: this server isn't group-1 leader, please retry")
	ErrMultipleGraphQLSchemaNodes = errors.New("found multiple nodes for GraphQL schema")
)

func UpdateGQLSchemaOverNetwork(ctx context.Context, req *pb.UpdateGraphQLSchemaRequest) (*pb.
	UpdateGraphQLSchemaResponse, error) {
	if isGroup1Leader() {
		return (&grpcWorker{}).UpdateGraphQLSchema(ctx, req)
	}

	pl := groups().Leader(1)
	if pl == nil {
		return nil, conn.ErrNoConnection
	}
	con := pl.Get()
	c := pb.NewWorkerClient(con)

	return c.UpdateGraphQLSchema(ctx, req)
}

func (w *grpcWorker) UpdateGraphQLSchema(ctx context.Context,
	req *pb.UpdateGraphQLSchemaRequest) (*pb.UpdateGraphQLSchemaResponse, error) {
	if !isGroup1Leader() {
		return nil, errUpdatingGQLSchemaOnNonGroup1Leader
	}

	// lock here so that only one request is served at a time by group 1 leader
	schemaLock.Lock()
	defer schemaLock.Unlock()

	var err error
	// TODO: what does readTs do here? can we share the same with other startTs as well for mutation
	// and alter? Can it be part of incoming req?
	// query the GraphQL schema node uid
	res, err := ProcessTaskOverNetwork(ctx, &pb.Query{
		Attr:    "dgraph.graphql.schema",
		SrcFunc: &pb.SrcFunction{Name: "has"},
		ReadTs:  State.GetTimestamp(true),
		First:   math.MaxInt32, // TODO: should we request just first 2, as that will also get error
	})
	if err != nil {
		return nil, err
	}

	// find if we need to create the node or can use the uid from existing node
	creatingNode := false
	var schemaNodeUid uint64
	uidMtrxLen := len(res.GetUidMatrix())
	if uidMtrxLen == 0 || (uidMtrxLen == 1 && len(res.GetUidMatrix()[0].GetUids()) == 0) {
		// if there was no schema node earlier, then need to assign a new uid for the node
		res, err := AssignUidsOverNetwork(ctx, &pb.Num{Val: 1})
		if err != nil {
			return nil, err
		}
		creatingNode = true
		schemaNodeUid = res.StartId
	} else if uidMtrxLen == 1 && len(res.GetUidMatrix()[0].GetUids()) == 1 {
		// if there was already a schema node, then just use the uid from that node
		schemaNodeUid = res.GetUidMatrix()[0].GetUids()[0]
	} else {
		// there seems to be multiple nodes for GraphQL schema, we should never reach here
		return nil, ErrMultipleGraphQLSchemaNodes
	}

	// prepare GraphQL schema mutation
	m := &pb.Mutations{
		StartTs: State.GetTimestamp(false),
		Edges: []*pb.DirectedEdge{
			{
				Entity:    schemaNodeUid,
				Attr:      "dgraph.graphql.schema",
				Value:     []byte(req.GraphqlSchema),
				ValueType: pb.Posting_STRING,
				Op:        pb.DirectedEdge_SET,
			},
		},
	}
	if creatingNode {
		m.Edges = append(m.Edges, &pb.DirectedEdge{
			Entity:    schemaNodeUid,
			Attr:      "dgraph.type",
			Value:     []byte("dgraph.graphql"),
			ValueType: pb.Posting_STRING,
			Op:        pb.DirectedEdge_SET,
		})
	}
	// mutate the GraphQL schema. As it is a reserved predicate, and we are in group 1,
	// so this call is gonna come back to all the group 1 servers only
	tctx, err := MutateOverNetwork(ctx, m)
	if err != nil {
		return nil, err
	}

	// perform dgraph schema alter
	_, err = MutateOverNetwork(ctx, &pb.Mutations{
		// TODO: why is startTs required here? need to understand
		StartTs: State.GetTimestamp(false),
		Schema:  req.DgraphSchema,
		Types:   req.DgraphTypes,
	})

	// commit or abort GraphQL schema mutation based on whether alter succeeded or not
	if err != nil {
		// abort the GraphQL schema mutation, don't care if error occurs during abort
		tctx.Aborted = true
		_, _ = CommitOverNetwork(ctx, tctx)
		return nil, err
	} else {
		// busy waiting for indexing to finish
		for schema.State().IndexingInProgress() {
			// return if context was Canceled or DeadlineExceeded
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			time.Sleep(time.Second * 2)
		}

		// TODO: check when do we receive badger update, on commit or on mutate?
		// commit the GraphQL schema mutation as alter succeeded
		_, err = CommitOverNetwork(ctx, tctx)
		if err != nil {
			// this is a problem, now we can't rollback the dgraph schema alter, so the system is
			// in an inconsistent state. So, lets just report the error asking the user to send the
			// GraphQL schema update again.
			return nil, errors.Wrap(err, "error occurred updating GraphQL schema, please retry")
		}
	}

	// return the uid of the GraphQL schema node
	return &pb.UpdateGraphQLSchemaResponse{Uid: schemaNodeUid}, nil
}

func isGroup1Leader() bool {
	return groups().ServesGroup(1) && groups().Node.AmLeader()
}
