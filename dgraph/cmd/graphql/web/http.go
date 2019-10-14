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
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

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

	return api.WithRequestID(recoveryHandler(commonHeaders(
		&graphqlHandler{
			schema:           schema,
			dgraphClient:     dgraphClient,
			queryRewriter:    queryRewriter,
			mutationRewriter: mutationRewriter,
		})))
}

// write chooses between the http response writer and gzip writer
// and sends the schema response using that.
func write(w http.ResponseWriter, rr *schema.Response, errMsg string, acceptGzip bool) {
	var out io.Writer = w

	// If the receiver accepts gzip, then we would update the writer
	// and send gzipped content instead.
	if acceptGzip {
		w.Header().Set("Content-Encoding", "gzip")
		gzw := gzip.NewWriter(w)
		defer gzw.Close()
		out = gzw
	}

	if _, err := rr.WriteTo(out); err != nil {
		glog.Error(errMsg, err)
	}
}

// ServeHTTP handles GraphQL queries and mutations that get resolved
// via GraphQL->Dgraph->GraphQL.  It writes a valid GraphQL JSON response
// to w.
func (gh *graphqlHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

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

	write(w, res, fmt.Sprintf("[%s]", api.RequestID(ctx)),
		strings.Contains(r.Header.Get("Accept-Encoding"), "gzip"))

}

func (gh *graphqlHandler) isValid() bool {
	return !(gh == nil || gh.schema == nil || gh.dgraphClient == nil ||
		gh.queryRewriter == nil || gh.mutationRewriter == nil)
}

type gzreadCloser struct {
	*gzip.Reader
	io.Closer
}

func (gz gzreadCloser) Close() error {
	err := gz.Reader.Close()
	if err != nil {
		return err
	}
	return gz.Closer.Close()
}

func (gh *graphqlHandler) resolverForRequest(r *http.Request) (*resolve.RequestResolver, error) {
	rr := resolve.New(gh.schema, gh.dgraphClient, gh.queryRewriter, gh.mutationRewriter)

	if r.Header.Get("Content-Encoding") == "gzip" {
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			return nil, errors.Wrap(err, "Unable to parse gzip")
		}
		r.Body = gzreadCloser{zr, r.Body}
	}

	switch r.Method {
	case http.MethodGet:
		query := r.URL.Query()
		rr.GqlReq = &schema.Request{}
		rr.GqlReq.Query = query.Get("query")
		rr.GqlReq.OperationName = query.Get("operationName")
		variables, ok := query["variables"]

		if ok {
			d := json.NewDecoder(strings.NewReader(variables[0]))
			d.UseNumber()

			if err := d.Decode(&rr.GqlReq.Variables); err != nil {
				return nil, errors.Wrap(err, "Not a valid GraphQL request body")
			}
		}
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
				return nil, errors.Wrap(err, "Not a valid GraphQL request body")
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

func commonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		x.AddCorsHeaders(w)
		w.Header().Set("Content-Type", "application/json")

		next.ServeHTTP(w, r)
	})
}

func recoveryHandler(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := api.RequestID(r.Context())
		defer api.PanicHandler(reqID,
			func(err error) {
				rr := schema.ErrorResponse(err, reqID)
				write(w, rr, fmt.Sprintf("[%s]", reqID),
					strings.Contains(r.Header.Get("Accept-Encoding"), "gzip"))
			})

		next.ServeHTTP(w, r)
	})
}
