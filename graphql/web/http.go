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
	"context"
	"encoding/json"
	"io"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dgryski/go-farm"
	"github.com/golang/glog"
	"github.com/graph-gophers/graphql-transport-ws/graphqlws"
	"go.opencensus.io/trace"
	"google.golang.org/grpc/peer"

	"github.com/dgraph-io/dgraph/graphql/api"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

// An IServeGraphQL can serve a GraphQL endpoint (currently only ons http)
type IServeGraphQL interface {

	// After ServeGQL is called, this IServeGraphQL serves the new resolvers.
	ServeGQL(resolver *resolve.RequestResolver)

	// HTTPHandler returns a http.Handler that serves GraphQL.
	HTTPHandler() http.Handler

	// Resolve processes a GQL Request using the correct resolver and returns a GQL Response
	Resolve(ctx context.Context, gqlReq *schema.Request) *schema.Response
}

type graphqlHandler struct {
	resolver *resolve.RequestResolver
	handler  http.Handler
}

// NewServer returns a new IServeGraphQL that can serve the given resolvers
func NewServer(resolver *resolve.RequestResolver) IServeGraphQL {
	gh := &graphqlHandler{resolver: resolver}
	gh.handler = recoveryHandler(commonHeaders(gh.Handler()))
	return gh
}

func (gh *graphqlHandler) HTTPHandler() http.Handler {
	return gh.handler
}

func (gh *graphqlHandler) ServeGQL(resolver *resolve.RequestResolver) {
	gh.resolver = resolver
}

func (gh *graphqlHandler) Resolve(ctx context.Context, gqlReq *schema.Request) *schema.Response {
	return gh.resolver.Resolve(ctx, gqlReq)
}

// write chooses between the http response writer and gzip writer
// and sends the schema response using that.
func write(w http.ResponseWriter, rr *schema.Response, acceptGzip bool) {
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
		glog.Error(err)
	}
}

type graphqlSubscription struct {
	graphqlHandler *graphqlHandler
}

func (gs *graphqlSubscription) Subscribe(
	ctx context.Context,
	document string,
	operationName string,
	variableValues map[string]interface{}) (payloads <-chan interface{},
	err error) {
	req := &schema.Request{
		OperationName: operationName,
		Query:         document,
		Variables:     variableValues,
	}
	ch := make(chan interface{}, 10)
	// TODO: @balajijinnah.
	// - Cancel the subscription if there a schema change.
	// - Batch same request in a one polling go routine.
	res := gs.graphqlHandler.Resolve(ctx, req)
	if len(res.Errors) != 0 {
		return nil, res.Errors
	}
	ch <- res.Output()
	prevHash := farm.Fingerprint64(res.Data.Bytes())

	// Poll the server for every one second.
	go func() {
		for {
			time.Sleep(time.Second)
			select {
			case <-ctx.Done():
				return
			default:
				res = gs.graphqlHandler.Resolve(ctx, req)
				if len(res.Errors) != 0 {
					ch <- res.Output()
					close(ch)
					return
				}
				hash := farm.Fingerprint64(res.Data.Bytes())
				if hash == prevHash {
					continue
				}
				prevHash = hash
				// Update the client if there is change.
				ch <- res.Output()
			}
		}
	}()
	return ch, ctx.Err()
}

func (gh *graphqlHandler) Handler() http.Handler {
	return graphqlws.NewHandlerFunc(&graphqlSubscription{
		graphqlHandler: gh,
	}, gh)
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

	ctx = x.AttachAccessJwt(ctx, r)

	if ip, port, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		// Add remote addr as peer info so that the remote address can be logged
		// inside Server.Login
		if intPort, convErr := strconv.Atoi(port); convErr == nil {
			ctx = peer.NewContext(ctx, &peer.Peer{
				Addr: &net.TCPAddr{
					IP:   net.ParseIP(ip),
					Port: intPort,
				},
			})
		}
	}

	var res *schema.Response
	gqlReq, err := getRequest(ctx, r)

	if err != nil {
		res = schema.ErrorResponse(err)
	} else {
		res = gh.resolver.Resolve(ctx, gqlReq)
	}

	write(w, res, strings.Contains(r.Header.Get("Accept-Encoding"), "gzip"))
}

func (gh *graphqlHandler) isValid() bool {
	return !(gh == nil || gh.resolver == nil)
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

func getRequest(ctx context.Context, r *http.Request) (*schema.Request, error) {
	gqlReq := &schema.Request{}

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
		gqlReq.Query = query.Get("query")
		gqlReq.OperationName = query.Get("operationName")
		variables, ok := query["variables"]
		if ok {
			d := json.NewDecoder(strings.NewReader(variables[0]))
			d.UseNumber()

			if err := d.Decode(&gqlReq.Variables); err != nil {
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
			if err = d.Decode(&gqlReq); err != nil {
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

	return gqlReq, nil
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
		defer api.PanicHandler(
			func(err error) {
				rr := schema.ErrorResponse(err)
				write(w, rr, strings.Contains(r.Header.Get("Accept-Encoding"), "gzip"))
			})

		next.ServeHTTP(w, r)
	})
}
