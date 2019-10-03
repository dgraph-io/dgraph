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

package graphql

import (
	"encoding/json"
	"mime"
	"net/http"
	"runtime/debug"

	"github.com/golang/glog"
	"go.opencensus.io/trace"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/api"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/dgraph"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/resolve"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
)

func recoveryHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			// This function makes sure that we recover from panics during execution of the request
			// and return appropriate error to the user.
			if err := recover(); err != nil {
				glog.Errorf("panic: %s while executing request with ID: %s, trace: %s", err,
					api.RequestID(r.Context()), string(debug.Stack()))
				rr := schema.ErrorResponsef("Internal Server Error")
				w.Header().Set("Content-Type", "application/json")
				if _, err := rr.WriteTo(w); err != nil {
					glog.Error(err)
				}
			}
		}()

		next.ServeHTTP(w, r)
	})
}

type graphqlHandler struct {
	dgraphClient *dgo.Dgraph
	schema       *ast.Schema
}

// ServeHTTP handles GraphQL queries and mutations that get resolved
// via GraphQL->Dgraph->GraphQL.  It writes a valid GraphQL JSON response
// to w.
func (gh *graphqlHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	ctx, span := trace.StartSpan(r.Context(), "handler")
	defer span.End()

	if !gh.isValid() {
		panic("graphqlHandler not initialised")
	}

	rh := gh.resolverForRequest(r)
	res := rh.Resolve(ctx)
	if _, err := res.WriteTo(w); err != nil {
		glog.Error(err)
	}
}

func (gh *graphqlHandler) isValid() bool {
	return !(gh == nil || gh.schema == nil || gh.dgraphClient == nil)
}

func (gh *graphqlHandler) resolverForRequest(r *http.Request) (rr *resolve.RequestResolver) {
	rr = resolve.New(
		schema.AsSchema(gh.schema),
		dgraph.AsDgraph(gh.dgraphClient),
		dgraph.NewQueryRewriter(),
		dgraph.NewMutationRewriter())

	switch r.Method {
	case http.MethodGet:
		// TODO: fill gqlReq in from parameters
		rr.WithError(gqlerror.Errorf("GraphQL on HTTP GET not yet implemented"))
		return
	case http.MethodPost:
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			rr.WithError(gqlerror.Errorf("Unable to parse media type: %s", err))
			return
		}

		switch mediaType {
		case "application/json":
			d := json.NewDecoder(r.Body)
			d.UseNumber()
			if err = d.Decode(&rr.GqlReq); err != nil {
				rr.WithError(
					gqlerror.Errorf("Not a valid GraphQL request body: %s", err))
				return
			}
		default:
			// https://graphql.org/learn/serving-over-http/#post-request says:
			// "A standard GraphQL POST request should use the application/json
			// content type ..."
			rr.WithError(gqlerror.Errorf(
				"Unrecognised Content-Type.  Please use application/json for GraphQL requests"))
			return
		}
	default:
		rr.WithError(gqlerror.Errorf(
			"Unrecognised request method.  Please use GET or POST for GraphQL requests"))
		return
	}
	return
}
