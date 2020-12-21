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
	"sync"
	"time"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/pkg/errors"
)

const (
	errGraphQLSchemaCommitFailed = "error occurred updating GraphQL schema, please retry"
	ErrGraphQLSchemaAlterFailed  = "succeeded in saving GraphQL schema but failed to alter Dgraph" +
		" schema - this indicates a bug in Dgraph schema generation. Please let us know by" +
		" filing an issue. Don't forget to post your old and new schemas in the issue description."

	GqlSchemaPred    = "dgraph.graphql.schema"
	gqlSchemaXidPred = "dgraph.graphql.xid"
	gqlSchemaXidVal  = "dgraph.graphql.schema"
)

var (
	schemaLock                                  sync.Mutex
	errUpdatingGraphQLSchemaOnNonGroupOneLeader = errors.New(
		"while updating GraphQL schema: this server isn't group-1 leader, please retry")
	ErrMultipleGraphQLSchemaNodes = errors.New("found multiple nodes for GraphQL schema")
)

// UpdateGQLSchemaOverNetwork sends the request to the group one leader for execution.
func UpdateGQLSchemaOverNetwork(ctx context.Context, req *pb.UpdateGraphQLSchemaRequest) (*pb.
	UpdateGraphQLSchemaResponse, error) {
	if isGroupOneLeader() {
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

// UpdateGraphQLSchema updates the GraphQL schema node with the new GraphQL schema,
// and then alters the dgraph schema. All this is done only on group one leader.
func (w *grpcWorker) UpdateGraphQLSchema(ctx context.Context,
	req *pb.UpdateGraphQLSchemaRequest) (*pb.UpdateGraphQLSchemaResponse, error) {
	if !isGroupOneLeader() {
		return nil, errUpdatingGraphQLSchemaOnNonGroupOneLeader
	}

	// lock here so that only one request is served at a time by group 1 leader
	schemaLock.Lock()
	defer schemaLock.Unlock()

	// query the GraphQL schema node uid
	res, err := ProcessTaskOverNetwork(ctx, &pb.Query{
		Attr:    GqlSchemaPred,
		SrcFunc: &pb.SrcFunction{Name: "has"},
		ReadTs:  req.StartTs,
		// there can only be one GraphQL schema node,
		// so querying two just to detect if this condition is ever violated
		First: 2,
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
		StartTs: req.StartTs,
		Edges: []*pb.DirectedEdge{
			{
				Entity:    schemaNodeUid,
				Attr:      GqlSchemaPred,
				Value:     []byte(req.GraphqlSchema),
				ValueType: pb.Posting_STRING,
				Op:        pb.DirectedEdge_SET,
			},
			{
				// if this server is no more the Group-1 leader and is mutating the GraphQL
				// schema node, also if concurrently another schema update is requested which is
				// being performed at the actual Group-1 leader, then mutating the xid with the
				// same value will cause one of the mutations to abort, because of the upsert
				// directive on xid. So, this way we make sure that even in this rare case there can
				// only be one server which is able to successfully update the GraphQL schema.
				Entity:    schemaNodeUid,
				Attr:      gqlSchemaXidPred,
				Value:     []byte(gqlSchemaXidVal),
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
	// commit the mutation here itself. This has two benefits:
	// 	1. If there was any concurrent request to update the GraphQL schema, then one of the two
	//	will fail here itself, and the alter for the failed one won't happen.
	// 	2. If the commit succeeds, then as alter takes some time to finish, so the badger
	//	notification for dgraph.graphql.schema predicate will reach all the alphas in the meantime,
	//	providing every alpha a chance to reflect the current GraphQL schema before the response is
	//	sent back to the user.
	if _, err = CommitOverNetwork(ctx, tctx); err != nil {
		return nil, errors.Wrap(err, errGraphQLSchemaCommitFailed)
	}

	// perform dgraph schema alter, if required. As the schema could be empty if it only has custom
	// types/queries/mutations.
	if len(req.DgraphPreds) != 0 && len(req.DgraphTypes) != 0 {
		if _, err = MutateOverNetwork(ctx, &pb.Mutations{
			StartTs: State.GetTimestamp(false), // StartTs must be provided
			Schema:  req.DgraphPreds,
			Types:   req.DgraphTypes,
		}); err != nil {
			return nil, errors.Wrap(err, ErrGraphQLSchemaAlterFailed)
		}
		// busy waiting for indexing to finish
		if err = WaitForIndexingOrCtxError(ctx, true); err != nil {
			return nil, err
		}
	}

	// return the uid of the GraphQL schema node
	return &pb.UpdateGraphQLSchemaResponse{Uid: schemaNodeUid}, nil
}

// WaitForIndexingOrCtxError does a busy wait for indexing to finish or the context to error out,
// if the input flag shouldWait is true. Otherwise, it just returns nil straight away.
// If the context errors, it returns that error.
func WaitForIndexingOrCtxError(ctx context.Context, shouldWait bool) error {
	for shouldWait {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !schema.State().IndexingInProgress() {
			break
		}
		time.Sleep(time.Second * 2)
	}
	return nil
}

// isGroupOneLeader returns true if the current server is the leader of Group One,
// it returns false otherwise.
func isGroupOneLeader() bool {
	return groups().ServesGroup(1) && groups().Node.AmLeader()
}
