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
	"sort"
	"strconv"

	dgoapi "github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/authorization"
	"github.com/dgraph-io/dgraph/graphql/dgraph"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	otrace "go.opencensus.io/trace"
)

const touchedUidsKey = "_total"

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
	Rewrite(ctx context.Context, m schema.Mutation) ([]*UpsertMutation, error)

	// FromMutationResult takes a GraphQL mutation and the results of a Dgraph
	// mutation and constructs a Dgraph query.  It's used to find the return
	// value from a GraphQL mutation - i.e. we've run the mutation indicated by m
	// now we need to query Dgraph to satisfy all the result fields in m.
	FromMutationResult(
		ctx context.Context,
		m schema.Mutation,
		assigned map[string]string,
		result map[string]interface{}) ([]*gql.GraphQuery, error)
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
	Query     []*gql.GraphQuery
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
type MutationResolverFunc func(ctx context.Context, m schema.Mutation) (*Resolved, bool)

// Resolve calls mr(ctx, mutation)
func (mr MutationResolverFunc) Resolve(ctx context.Context, m schema.Mutation) (*Resolved, bool) {
	return mr(ctx, m)
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

// mutationResolver can resolve a single GraphQL mutation field
type dgraphResolver struct {
	mutationRewriter MutationRewriter
	executor         DgraphExecutor
	resultCompleter  ResultCompleter
}

func (mr *dgraphResolver) Resolve(ctx context.Context, m schema.Mutation) (*Resolved, bool) {
	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "resolveMutation")
	defer stop()
	if span != nil {
		span.Annotatef(nil, "mutation alias: [%s] type: [%s]", m.Alias(), m.MutationType())
	}

	resolverTrace := &schema.ResolverTrace{
		Path:       []interface{}{m.ResponseName()},
		ParentType: "Mutation",
		FieldName:  m.ResponseName(),
		ReturnType: m.Type().String(),
	}
	timer := newtimer(ctx, &resolverTrace.OffsetDuration)
	timer.Start()
	defer timer.Stop()

	resolved, success := mr.rewriteAndExecute(ctx, m)
	mr.resultCompleter.Complete(ctx, resolved)
	resolverTrace.Dgraph = resolved.Extensions.Tracing.Execution.Resolvers[0].Dgraph
	resolved.Extensions.Tracing.Execution.Resolvers[0] = resolverTrace
	return resolved, success
}

func getNumUids(m schema.Mutation, a map[string]string, r map[string]interface{}) int {
	switch m.MutationType() {
	case schema.AddMutation:
		return len(a)
	default:
		mutated := extractMutated(r, m.Name())
		return len(mutated)
	}
}

func (mr *dgraphResolver) rewriteAndExecute(ctx context.Context,
	mutation schema.Mutation) (*Resolved, bool) {
	var mutResp *dgoapi.Response
	commit := false

	defer func() {
		if !commit && mutResp != nil && mutResp.Txn != nil {
			mutResp.Txn.Aborted = true
			err := mr.executor.CommitOrAbort(ctx, mutResp.Txn)
			if err != nil {
				glog.Errorf("Error occured while aborting transaction: %s", err)
			}
		}
	}()

	dgraphMutationDuration := &schema.LabeledOffsetDuration{Label: "mutation"}
	dgraphQueryDuration := &schema.LabeledOffsetDuration{Label: "query"}
	ext := &schema.Extensions{
		Tracing: &schema.Trace{
			Execution: &schema.ExecutionTrace{
				Resolvers: []*schema.ResolverTrace{
					{
						Dgraph: []*schema.LabeledOffsetDuration{
							dgraphMutationDuration,
							dgraphQueryDuration,
						},
					},
				},
			},
		},
	}

	emptyResult := func(err error) *Resolved {
		return &Resolved{
			Data:       map[string]interface{}{mutation.DgraphAlias(): nil},
			Field:      mutation,
			Err:        err,
			Extensions: ext,
		}
	}

	upserts, err := mr.mutationRewriter.Rewrite(ctx, mutation)
	if err != nil {
		return emptyResult(schema.GQLWrapf(err, "couldn't rewrite mutation %s", mutation.Name())),
			resolverFailed
	}
	if len(upserts) == 0 {
		return &Resolved{
			Data: map[string]interface{}{
				mutation.DgraphAlias(): map[string]interface{}{
					schema.NumUid:                       0,
					mutation.QueryField().DgraphAlias(): nil,
				}},
			Field:      mutation,
			Err:        nil,
			Extensions: ext,
		}, resolverSucceeded
	}

	result := make(map[string]interface{})
	req := &dgoapi.Request{}
	newNodes := make(map[string]schema.Type)

	mutationTimer := newtimer(ctx, &dgraphMutationDuration.OffsetDuration)
	mutationTimer.Start()

	for _, upsert := range upserts {
		req.Query = dgraph.AsString(upsert.Query)
		req.Mutations = upsert.Mutations
		mutResp, err = mr.executor.Execute(ctx, req)
		if err != nil {
			gqlErr := schema.GQLWrapLocationf(
				err, mutation.Location(), "mutation %s failed", mutation.Name())
			return emptyResult(gqlErr), resolverFailed

		}

		ext.TouchedUids += mutResp.GetMetrics().GetNumUids()[touchedUidsKey]
		if req.Query != "" && len(mutResp.GetJson()) != 0 {
			if err := json.Unmarshal(mutResp.GetJson(), &result); err != nil {
				return emptyResult(
						schema.GQLWrapf(err, "Couldn't unmarshal response from Dgraph mutation")),
					resolverFailed
			}
		}

		copyTypeMap(upsert.NewNodes, newNodes)
	}
	mutationTimer.Stop()

	authErr := authorizeNewNodes(ctx, mutResp.Uids, newNodes, mr.executor, mutResp.Txn)
	if authErr != nil {
		return emptyResult(schema.GQLWrapf(authErr, "mutation failed")), resolverFailed
	}

	var errs error
	dgQuery, err := mr.mutationRewriter.FromMutationResult(ctx, mutation, mutResp.GetUids(), result)
	errs = schema.AppendGQLErrs(errs, schema.GQLWrapf(err,
		"couldn't rewrite query for mutation %s", mutation.Name()))
	if dgQuery == nil && err != nil {
		return emptyResult(errs), resolverFailed
	}

	err = mr.executor.CommitOrAbort(ctx, mutResp.Txn)
	if err != nil {
		return emptyResult(
				schema.GQLWrapf(authErr, "mutation failed, couldn't commit transaction")),
			resolverFailed
	}
	commit = true

	queryTimer := newtimer(ctx, &dgraphQueryDuration.OffsetDuration)
	queryTimer.Start()
	qryResp, err := mr.executor.Execute(ctx, &dgoapi.Request{Query: dgraph.AsString(dgQuery),
		ReadOnly: true})
	queryTimer.Stop()

	errs = schema.AppendGQLErrs(errs, schema.GQLWrapf(err,
		"couldn't rewrite query for mutation %s", mutation.Name()))

	ext.TouchedUids += qryResp.GetMetrics().GetNumUids()[touchedUidsKey]
	numUids := getNumUids(mutation, mutResp.Uids, result)

	var qryResult []byte
	if mutation.MutationType() == schema.DeleteMutation && mutation.QueryField().SelectionSet() != nil {
		qryResult = mutResp.GetJson()
	} else {
		qryResult = qryResp.GetJson()
	}

	resolved := completeDgraphResult(ctx, mutation.QueryField(), qryResult, errs)
	if resolved.Data == nil && resolved.Err != nil {
		return &Resolved{
			Data: map[string]interface{}{
				mutation.DgraphAlias(): map[string]interface{}{
					schema.NumUid:                       numUids,
					mutation.QueryField().DgraphAlias(): nil,
				}},
			Field:      mutation,
			Err:        err,
			Extensions: ext,
		}, resolverSucceeded
	}

	if resolved.Data == nil {
		resolved.Data = map[string]interface{}{}
	}

	dgRes := resolved.Data.(map[string]interface{})
	dgRes[schema.NumUid] = numUids
	resolved.Data = map[string]interface{}{mutation.DgraphAlias(): dgRes}
	resolved.Field = mutation
	resolved.Extensions = ext

	return resolved, resolverSucceeded
}

// deleteCompletion returns `{ "msg": "Deleted" }`
func deleteCompletion() CompletionFunc {
	return CompletionFunc(func(ctx context.Context, resolved *Resolved) {
		if fld, ok := resolved.Data.(map[string]interface{}); ok {
			if rsp, ok := fld[resolved.Field.Name()].(map[string]interface{}); ok {
				rsp["msg"] = "Deleted"
				if rsp[schema.NumUid] == 0 {
					rsp["msg"] = "No nodes were deleted"
				}
			}
		}
	})
}

// authorizeNewNodes takes the new nodes (uids) actually created by a GraphQL mutation and
// the types that mutation rewriting expects those nodes to be (newNodeTypes) and checks if
// the JWT that came in with the request is authorized to create those nodes.  We can't check
// this before the mutation, because the nodes aren't linked into the graph yet.
//
// We group the nodes into their types, generate the authorization add rules for that type
// and then check that the authorized nodes for each type is equal to the nodes created
// for that type by performing an authorization query to Dgraph as part of the ongoing
// transaction (txn).  If the authorization query returns fewer nodes than we created, some
// of the new nodes failed the auth rules.
func authorizeNewNodes(
	ctx context.Context,
	uids map[string]string,
	newNodeTypes map[string]schema.Type,
	queryExecutor DgraphExecutor,
	txn *dgoapi.TxnContext) error {

	customClaims, err := authorization.ExtractCustomClaims(ctx)
	if err != nil {
		return schema.GQLWrapf(err, "authorization failed")
	}
	authVariables := customClaims.AuthVariables
	newRw := &authRewriter{
		authVariables: authVariables,
		varGen:        NewVariableGenerator(),
		selector:      addAuthSelector,
		hasAuthRules:  true,
	}

	// Collect all the newly created nodes in type groups

	newByType := make(map[string][]uint64)
	namesToType := make(map[string]schema.Type)
	for nodeName, nodeTyp := range newNodeTypes {
		if uidStr, created := uids[nodeName]; created {
			uid, err := strconv.ParseUint(uidStr, 0, 64)
			if err != nil {
				return schema.GQLWrapf(err, "authorization failed")
			}
			if nodeTyp.ListType() != nil {
				nodeTyp = nodeTyp.ListType()
			}
			namesToType[nodeTyp.Name()] = nodeTyp
			newByType[nodeTyp.Name()] = append(newByType[nodeTyp.Name()], uid)
		}
	}

	// sort to get a consistent query rewriting
	var createdTypes []string
	for typeName := range newByType {
		createdTypes = append(createdTypes, typeName)
	}
	sort.Strings(createdTypes)

	// Write auth queries for each set of node types

	var needsAuth []string
	authQrys := make(map[string][]*gql.GraphQuery)
	for _, typeName := range createdTypes {
		typ := namesToType[typeName]
		varName := newRw.varGen.Next(typ, "", "", false)
		newRw.varName = varName
		newRw.parentVarName = typ.Name() + "Root"
		authQueries, authFilter := newRw.rewriteAuthQueries(typ)

		rn := newRw.selector(typ)
		rbac := rn.EvaluateStatic(newRw.authVariables)

		if rbac == schema.Negative {
			return x.GqlErrorf("authorization failed")
		}

		if rbac == schema.Positive {
			continue
		}

		if len(authQueries) == 0 {
			continue
		}

		// Generate query blocks like this for each node type
		//
		// Todo(func: uid(Todo1)) @filter(uid(Todo2) AND uid(Todo3)) { uid }
		// Todo1 as var(func: uid(...new uids of this type...) )
		// Todo2 as var(func: uid(Todo1)) @cascade { ...auth query 1... }
		// Todo3 as var(func: uid(Todo1)) @cascade { ...auth query 2... }

		typQuery := &gql.GraphQuery{
			Attr: typ.Name(),
			Func: &gql.Function{
				Name: "uid",
				Args: []gql.Arg{{Value: varName}}},
			Filter:   authFilter,
			Children: []*gql.GraphQuery{{Attr: "uid"}}}

		nodes := newByType[typeName]
		sort.Slice(nodes, func(i, j int) bool { return nodes[i] < nodes[j] })
		varQry := &gql.GraphQuery{
			Var:  varName,
			Attr: "var",
			Func: &gql.Function{
				Name: "uid",
				UID:  nodes,
			},
		}

		needsAuth = append(needsAuth, typeName)
		authQrys[typeName] = append([]*gql.GraphQuery{typQuery, varQry}, authQueries...)

	}

	if len(needsAuth) == 0 {
		// no auth to apply
		return nil
	}

	// create the query in order so we get a stable query
	sort.Strings(needsAuth)
	var qs []*gql.GraphQuery
	for _, typeName := range needsAuth {
		qs = append(qs, authQrys[typeName]...)
	}

	resp, errs := queryExecutor.Execute(ctx,
		&dgoapi.Request{
			Query:   dgraph.AsString(qs),
			StartTs: txn.GetStartTs(),
		})
	if errs != nil || len(resp.Json) == 0 {
		return x.GqlErrorf("authorization request failed")
	}

	authResult := make(map[string]interface{})
	if err := json.Unmarshal(resp.Json, &authResult); err != nil {
		return x.GqlErrorf("authorization checking failed")
	}

	for _, typeName := range needsAuth {
		check, ok := authResult[typeName]
		if !ok || check == nil {
			// We needed auth on this type, but it wasn't even in the response.  That
			// means Dgraph found no matching nodes and returned nothing for this field.
			// So all the nodes failed auth.

			// FIXME: what do we actually want to return to users when auth failed?
			// Is this too much?
			return x.GqlErrorf("authorization failed")
		}

		foundUIDs, ok := check.([]interface{})
		if !ok {
			return x.GqlErrorf("authorization failed")
		}

		if len(newByType[typeName]) != len(foundUIDs) {
			// Some of the created nodes passed auth and some failed.
			return x.GqlErrorf("authorization failed")
		}
	}

	// By now either there were no types that needed auth, or all nodes passed the
	// auth checks.  So the mutation as a whole passed authorization.

	return nil
}
