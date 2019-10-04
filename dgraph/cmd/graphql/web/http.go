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

package web

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"

	"github.com/golang/glog"
	"go.opencensus.io/trace"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/api"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/dgraph"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/resolve"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

type graphqlHandler struct {
	schema           schema.Schema
	dgraphClient     dgraph.Client
	queryRewriter    dgraph.QueryRewriter
	mutationRewriter dgraph.MutationRewriter
}

// GraphQLHTTPHandler returns a http.Handler that serves GraphQL.
func GraphQLHTTPHandler(
	schema schema.Schema,
	dgraphClient dgraph.Client,
	queryRewriter dgraph.QueryRewriter,
	mutationRewriter dgraph.MutationRewriter) http.Handler {

	return api.WithRequestID(recoveryHandler(
		&graphqlHandler{
			schema:           schema,
			dgraphClient:     dgraphClient,
			queryRewriter:    queryRewriter,
			mutationRewriter: mutationRewriter,
		}))
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

	var res *schema.Response
	rh, err := gh.resolverForRequest(r)
	if err != nil {
		res = schema.ErrorResponse(err, api.RequestID(ctx))
	} else {
		res = rh.Resolve(ctx)
	}

	if _, err := res.WriteTo(w); err != nil {
		glog.Error(fmt.Sprintf("[%s]", api.RequestID(ctx)), err)
	}
}

func (gh *graphqlHandler) isValid() bool {
	return !(gh == nil || gh.schema == nil || gh.dgraphClient == nil ||
		gh.queryRewriter == nil || gh.mutationRewriter == nil)
}

func (gh *graphqlHandler) resolverForRequest(r *http.Request) (*resolve.RequestResolver, error) {
	rr := resolve.New(gh.schema, gh.dgraphClient, gh.queryRewriter, gh.mutationRewriter)

	switch r.Method {
	case http.MethodGet:
		// TODO: fill gqlReq in from parameters
		return nil, errors.New("GraphQL on HTTP GET not yet implemented")
	case http.MethodPost:
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			return nil, errors.Wrap(err, "unable to parse media type")
		}

		switch mediaType {
		case "application/json":
			d := json.NewDecoder(r.Body)
			d.UseNumber()
			if err = d.Decode(&rr.GqlReq); err != nil {
				return nil, errors.Wrap(err, "not a valid GraphQL request body")
			}
		default:
			// https://graphql.org/learn/serving-over-http/#post-request says:
			// "A standard GraphQL POST request should use the application/json
			// content type ..."
			return nil, errors.New(
				"Unrecognised Content-Type.  Please use application/json for GraphQL requests")
		}
	default:
		return nil,
			errors.New("Unrecognised request method.  Please use GET or POST for GraphQL requests")
	}

	return rr, nil
}

func recoveryHandler(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := api.RequestID(r.Context())
		defer api.PanicHandler(reqID,
			func(err error) {
				rr := schema.ErrorResponse(err, reqID)
				w.Header().Set("Content-Type", "application/json")
				if _, err = rr.WriteTo(w); err != nil {
					glog.Errorf("[%s] %s", reqID, err)
				}
			})

		next.ServeHTTP(w, r)
	})
}
