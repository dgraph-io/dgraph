/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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
	"sort"
	"sync"
	"time"

	"google.golang.org/grpc/metadata"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

const (
	errLambdaScriptCommitFailed = "error occurred updating Lambda Script, please retry"
	ErrLambdaScriptAlterFailed  = "succeeded in saving LambdaScript but failed to alter Dgraph schema - " +
		"GraphQL layer may exhibit unexpected behaviour, reapplying the old Lambda script may prevent any issues"

	LambdaPred    = "dgraph.lambda.script"
	lambdaXidPred = "dgraph.lambda.xid"
	lambdaXidVal  = "dgraph.lambda.script"
)

var (
	scriptLock                                 sync.Mutex
	errUpdatingLambdaScriptOnNonGroupOneLeader = errors.New(
		"while updating Lambda Script: this server isn't group-1 leader, please retry")
	ErrMultipleLambdaScriptNodes = errors.New("found multiple nodes for Lambda Script")
)

// UpdateLambdaScriptOverNetwork sends the request to the group one leader for execution.
func UpdateLambdaScriptOverNetwork(ctx context.Context, req *pb.UpdateLambdaScriptRequest) (*pb.
	UpdateLambdaScriptResponse, error) {
	if isGroupOneLeader() {
		return (&grpcWorker{}).UpdateLambdaScript(ctx, req)
	}

	pl := groups().Leader(1)
	if pl == nil {
		return nil, conn.ErrNoConnection
	}
	con := pl.Get()
	c := pb.NewWorkerClient(con)

	// pass on the incoming metadata to the group-1 leader
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	return c.UpdateLambdaScript(ctx, req)
}

// TODO: Refactor the logic from worker/graphql_schema.go
// UpdateLambdaScript updates the lambda script node with the new lambda script.
// This is done only on group one leader.
func (w *grpcWorker) UpdateLambdaScript(ctx context.Context,
	req *pb.UpdateLambdaScriptRequest) (*pb.UpdateLambdaScriptResponse, error) {
	if !isGroupOneLeader() {
		return nil, errUpdatingLambdaScriptOnNonGroupOneLeader
	}

	ctx = x.AttachJWTNamespace(ctx)
	namespace, err := x.ExtractNamespace(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "While updating lambda script")
	}

	waitStart := time.Now()

	// lock here so that only one request is served at a time by group 1 leader
	scriptLock.Lock()
	defer scriptLock.Unlock()

	waitDuration := time.Since(waitStart)
	if waitDuration > 500*time.Millisecond {
		glog.Warningf("Lambda script update for namespace %d waited for %s as another script"+
			" update was in progress.", namespace, waitDuration.String())
	}

	// query the Lambda script node uid
	res, err := ProcessTaskOverNetwork(ctx, &pb.Query{
		Attr:    x.NamespaceAttr(namespace, LambdaPred),
		SrcFunc: &pb.SrcFunction{Name: "has"},
		ReadTs:  req.StartTs,
		// there can only be one Lambda Script node,
		// so querying two just to detect if this condition is ever violated
		First: 2,
	})
	if err != nil {
		return nil, err
	}

	// find if we need to create the node or can use the uid from existing node
	creatingNode := false
	var scriptNodeUid uint64
	uidMtrxLen := len(res.GetUidMatrix())
	c := codec.ListCardinality(res.GetUidMatrix()[0])
	if uidMtrxLen == 0 || (uidMtrxLen == 1 && c == 0) {
		// if there was no script node earlier, then need to assign a new uid for the node
		res, err := AssignUidsOverNetwork(ctx, &pb.Num{Val: 1, Type: pb.Num_UID})
		if err != nil {
			return nil, err
		}
		creatingNode = true
		scriptNodeUid = res.StartId
	} else if uidMtrxLen == 1 && c == 1 {
		// if there was already a script node, then just use the uid from that node
		scriptNodeUid = codec.GetUids(res.GetUidMatrix()[0])[0]
	} else {
		// there seems to be multiple nodes for Lambda script,Ideally we should never reach here
		// But if by any bug we reach here then return the script node which is added last
		uidList := codec.GetUids(res.GetUidMatrix()[0])
		sort.Slice(uidList, func(i, j int) bool {
			return uidList[i] < uidList[j]
		})
		glog.Errorf("Multiple script node found, using the last one")
		scriptNodeUid = uidList[len(uidList)-1]
	}

	// prepare Grap mutation
	m := &pb.Mutations{
		StartTs: req.StartTs,
		Edges: []*pb.DirectedEdge{
			{
				Entity:    scriptNodeUid,
				Attr:      x.NamespaceAttr(namespace, LambdaPred),
				Value:     []byte(req.LambdaScript),
				ValueType: pb.Posting_STRING,
				Op:        pb.DirectedEdge_SET,
			},
			{
				// if this server is no more the Group-1 leader and is mutating the GraphQL
				// schema node, also if concurrently another schema update is requested which is
				// being performed at the actual Group-1 leader, then mutating the xid with the
				// same value will cause one of the mutations to abort, because of the upsert
				// directive on xid. So, this way we make sure that even in this rare case there can
				// only be one server which is able to successfully update the Lambda script.
				Entity:    scriptNodeUid,
				Attr:      x.NamespaceAttr(namespace, lambdaXidPred),
				Value:     []byte(lambdaXidVal),
				ValueType: pb.Posting_STRING,
				Op:        pb.DirectedEdge_SET,
			},
		},
	}
	if creatingNode {
		m.Edges = append(m.Edges, &pb.DirectedEdge{
			Entity:    scriptNodeUid,
			Attr:      x.NamespaceAttr(namespace, "dgraph.type"),
			Value:     []byte("dgraph.lambda"),
			ValueType: pb.Posting_STRING,
			Op:        pb.DirectedEdge_SET,
		})
	}
	// mutate the Lambda script. As it is a reserved predicate, and we are in group 1,
	// so this call is gonna come back to all the group 1 servers only
	tctx, err := MutateOverNetwork(ctx, m)
	if err != nil {
		return nil, err
	}
	// commit the mutation here itself. This has two benefits:
	// 	1. If there was any concurrent request to update the Lambda script, then one of the two
	//	will fail here itself, and the alter for the failed one won't happen.
	// TODO: Fix the comment.
	// 	2. If the commit succeeds, then as alter takes some time to finish, so the badger
	//	notification for dgraph.lambda.script predicate will reach all the alphas in the meantime,
	//	providing every alpha a chance to reflect the current Lambda script before the response is
	//	sent back to the user.
	if _, err = CommitOverNetwork(ctx, tctx); err != nil {
		return nil, errors.Wrap(err, errLambdaScriptCommitFailed)
	}

	// return the uid of the Lambda script node
	return &pb.UpdateLambdaScriptResponse{Uid: scriptNodeUid}, nil
}
