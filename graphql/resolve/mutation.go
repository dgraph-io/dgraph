/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package resolve

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	dgoapi "github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/dgraph"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	otrace "go.opencensus.io/trace"
)

// Mutations come in like this with variables:
//
// mutation themutation($post: PostInput!) {
//   addPost(input: $post) { ... some query ...}
// }
// - with variable payload
// { "post":
//   { "title": "My Post",
//     "author": { authorID: 0x123 },
//     ...
//   }
// }
//
//
// Or, like this with the payload in the mutation arguments
//
// mutation themutation {
//   addPost(input: { title: ... }) { ... some query ...}
// }
//
//
// Either way we build up a Dgraph json mutation to add the object
//
// For now, all mutations are only 1 level deep (cause of how we build the
// input objects) and only create a single node (again cause of inputs)

// A MutationResolver can resolve a single mutation.
type MutationResolver interface {
	Resolve(ctx context.Context, mutation schema.Mutation) (*Resolved, bool)
}

// A MutationRewriter can transform a GraphQL mutation into a Dgraph mutation and
// can build a Dgraph gql.GraphQuery to follow a GraphQL mutation.
//
// Mutations come in like:
//
// mutation addAuthor($auth: AuthorInput!) {
//   addAuthor(input: $auth) {
// 	   author {
// 	     id
// 	     name
// 	   }
//   }
// }
//
// Where `addAuthor(input: $auth)` implies a mutation that must get run - written
// to a Dgraph mutation by Rewrite.  The GraphQL following `addAuthor(...)`implies
// a query to run and return the newly created author, so the
// mutation query rewriting is dependent on the context set up by the result of
// the mutation.
type MutationRewriter interface {
	// Rewrite rewrites GraphQL mutation m into a Dgraph mutation - that could
	// be as simple as a single DelNquads, or could be a Dgraph upsert mutation
	// with a query and multiple mutations guarded by conditions.
	Rewrite(m schema.Mutation) (*gql.GraphQuery, []*dgoapi.Mutation, error)

	// FromMutationResult takes a GraphQL mutation and the results of a Dgraph
	// mutation and constructs a Dgraph query.  It's used to find the return
	// value from a GraphQL mutation - i.e. we've run the mutation indicated by m
	// now we need to query Dgraph to satisfy all the result fields in m.
	FromMutationResult(
		m schema.Mutation,
		assigned map[string]string,
		result map[string]interface{}) (*gql.GraphQuery, error)
}

// A DgraphExecutor can execute a mutation and returns the request response and any errors.
type DgraphExecutor interface {
	// Execute performs the actual mutation and returns a Dgraph response. If an error
	// occurs, that indicates that the execution failed in some way significant enough
	// way as to not continue processing this mutation or others in the same request.
	Execute(ctx context.Context, req *dgoapi.Request) (*dgoapi.Response, error)
	CommitOrAbort(ctx context.Context, tc *dgoapi.TxnContext) error
}

// An UpsertMutation is the query and mutations needed for a Dgraph upsert.
// The node types is a blank node name -> Type mapping of nodes that could
// be created by the upsert.
type UpsertMutation struct {
	Query     *gql.GraphQuery
	Mutations []*dgoapi.Mutation
	NewNodes  map[string]schema.Type
}

// DgraphExecutorFunc is an adapter that allows us to compose dgraph execution and
// build a QueryExecuter from a function.  Based on the http.HandlerFunc pattern.
type DgraphExecutorFunc func(ctx context.Context, req *dgoapi.Request) (*dgoapi.Response, error)

// Execute calls qe(ctx, query)
func (ex DgraphExecutorFunc) Execute(
	ctx context.Context,
	req *dgoapi.Request) (*dgoapi.Response, error) {

	return ex(ctx, req)
}

// MutationResolverFunc is an adapter that allows to build a MutationResolver from
// a function.  Based on the http.HandlerFunc pattern.
type MutationResolverFunc func(ctx context.Context, mutation schema.Mutation) (*Resolved, bool)

// MutationExecutionFunc is an adapter that allows us to compose mutation execution and build a
// MutationExecuter from a function.  Based on the http.HandlerFunc pattern.
type MutationExecutionFunc func(
	ctx context.Context,
	query *gql.GraphQuery,
	mutations []*dgoapi.Mutation) (map[string]string, map[string][]string, error)

// Resolve calls mr(ctx, mutation)
func (mr MutationResolverFunc) Resolve(
	ctx context.Context,
	mutation schema.Mutation) (*Resolved, bool) {

	return mr(ctx, mutation)
}

// Mutate calls me(ctx, query, mutations)
func (me MutationExecutionFunc) Mutate(
	ctx context.Context,
	query *gql.GraphQuery,
	mutations []*dgoapi.Mutation) (map[string]string, map[string][]string, error) {
	return me(ctx, query, mutations)
}

// NewDgraphResolver creates a new mutation resolver.  The resolver runs the pipeline:
// 1) rewrite the mutation using mr (return error if failed)
// 2) execute the mutation with me (return error if failed)
// 3) write a query for the mutation with mr (return error if failed)
// 4) execute the query with qe (return error if failed)
// 5) process the result with rc
func NewDgraphResolver(
	mr MutationRewriter,
	ex DgraphExecutor,
	rc ResultCompleter) MutationResolver {
	return &dgraphResolver{
		mutationRewriter: mr,
		executor:         ex,
		resultCompleter:  rc,
	}
}

// dgraphResolver can resolve a single GraphQL mutation field
type dgraphResolver struct {
	mutationRewriter MutationRewriter
	executor         DgraphExecutor
	resultCompleter  ResultCompleter

	numUids int
}

func (mr *dgraphResolver) Resolve(
	ctx context.Context, mutation schema.Mutation) (*Resolved, bool) {

	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "resolveMutation")
	defer stop()
	if span != nil {
		span.Annotatef(nil, "mutation alias: [%s] type: [%s]", mutation.Alias(),
			mutation.MutationType())
	}

	res, success, err := mr.rewriteAndExecute(ctx, mutation)

	completed, err := mr.resultCompleter.Complete(ctx, mutation.QueryField(), res, err)

	selSets := mutation.SelectionSet()
	for _, selSet := range selSets {
		if selSet.Name() != schema.NumUid {
			continue
		}

		s := string(completed)
		switch {
		case strings.Contains(s, schema.NumUid):
			completed = []byte(strings.ReplaceAll(s, fmt.Sprintf(`"%s": null`,
				schema.NumUid), fmt.Sprintf(`"%s": %d`, schema.NumUid,
				mr.numUids)))

		case s[len(s)-1] == '}':
			completed = []byte(fmt.Sprintf(`%s, "%s": %d}`, s[:len(s)-1],
				schema.NumUid, mr.numUids))

		default:
			completed = []byte(fmt.Sprintf(`%s, "%s": %d`, s,
				schema.NumUid, mr.numUids))
		}
		break
	}

	return &Resolved{
		Data: completed,
		Err:  err,
	}, success
}

func (mr *dgraphResolver) getNumUids(mutation schema.Mutation, assigned map[string]string,
	result map[string]interface{}) {
	switch mr.mutationRewriter.(type) {
	case *AddRewriter:
		mr.numUids = len(assigned)

	default:
		mutated := extractMutated(result, mutation.ResponseName())
		mr.numUids = len(mutated)
	}
}

func (mr *dgraphResolver) rewriteAndExecute(
	ctx context.Context, mutation schema.Mutation) ([]byte, bool, error) {
	var mutResp *dgoapi.Response
	commit := false

	defer func() {
		if !commit && mutResp != nil && mutResp.Txn != nil {
			mutResp.Txn.Aborted = true
			err := mr.executor.CommitOrAbort(ctx, mutResp.Txn)
			glog.Errorf("Error occured while aborting transaction: %f", err)
		}
	}()

	query, mutations, err := mr.mutationRewriter.Rewrite(mutation)
	if err != nil {
		return nil, resolverFailed,
			schema.GQLWrapf(err, "couldn't rewrite mutation %s", mutation.Name())
	}

	req := &dgoapi.Request{
		Query:     dgraph.AsString(query),
		Mutations: mutations,
	}

	mutResp, err = mr.executor.Execute(ctx, req)
	if err != nil {
		return nil, resolverFailed,
			schema.GQLWrapLocationf(err, mutation.Location(), "mutation %s failed", mutation.Name())
	}

	assigned := mutResp.GetUids()
	result := make(map[string]interface{})
	if req.Query != "" && len(mutResp.GetJson()) != 0 {
		if err := json.Unmarshal(mutResp.GetJson(), &result); err != nil {
			return nil, resolverFailed,
				schema.GQLWrapLocationf(err, mutation.Location(), "mutation %s failed", mutation.Name())
		}
	}

	mr.getNumUids(mutation, assigned, result)
	var errs error
	dgQuery, err := mr.mutationRewriter.FromMutationResult(mutation, assigned, result)
	errs = schema.AppendGQLErrs(errs, schema.GQLWrapf(err,
		"couldn't rewrite query for mutation %s", mutation.Name()))

	if dgQuery == nil && err != nil {
		return nil, resolverFailed, errs
	}

	err = mr.executor.CommitOrAbort(ctx, mutResp.Txn)
	if err != nil {
		return nil, resolverFailed,
			schema.GQLWrapLocationf(err, mutation.Location(), "mutation %s failed", mutation.Name())
	}
	commit = true

	resp, err := mr.executor.Execute(ctx,
		&dgoapi.Request{
			Query:    dgraph.AsString(dgQuery),
			ReadOnly: true,
		})

	errs = schema.AppendGQLErrs(errs, schema.GQLWrapf(err,
		"couldn't rewrite query for mutation %s", mutation.Name()))

	return resp.GetJson(), resolverSucceeded, errs
}

// deleteCompletion returns `{ "msg": "Deleted" }`
// FIXME: after upsert mutations changes are done, it will return info about
// the result of a deletion.
func deleteCompletion() CompletionFunc {
	return CompletionFunc(func(
		ctx context.Context, field schema.Field, result []byte, err error) ([]byte, error) {

		if field.Name() == "msg" {
			return []byte(`{ "msg": "Deleted" }`), err
		}

		return []byte(fmt.Sprintf(`{ "%s": null }`, schema.NumUid)), err
	})
}
