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
	"github.com/dgrijalva/jwt-go/v4"
	"google.golang.org/grpc/metadata"
	"strconv"
	"time"

	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"strings"

	"github.com/dgraph-io/dgraph/graphql/api"
	"github.com/dgraph-io/dgraph/graphql/authorization"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/graphql/subscription"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/graphql-transport-ws/graphqlws"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

type Headerkey string

const (
	touchedUidsHeader = "Graphql-TouchedUids"
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
	poller   *subscription.Poller
}

// NewServer returns a new IServeGraphQL that can serve the given resolvers
func NewServer(schemaEpoch *uint64, resolver *resolve.RequestResolver) IServeGraphQL {
	gh := &graphqlHandler{
		resolver: resolver,
		poller:   subscription.NewPoller(schemaEpoch, resolver),
	}
	gh.handler = recoveryHandler(commonHeaders(gh.Handler()))
	return gh
}

func (gh *graphqlHandler) HTTPHandler() http.Handler {
	return gh.handler
}

func (gh *graphqlHandler) ServeGQL(resolver *resolve.RequestResolver) {
	gh.poller.UpdateResolver(resolver)
	gh.resolver = resolver
}

func (gh *graphqlHandler) Resolve(ctx context.Context, gqlReq *schema.Request) *schema.Response {
	return gh.resolver.Resolve(ctx, gqlReq)
}

// write chooses between the http response writer and gzip writer
// and sends the schema response using that.
func write(w http.ResponseWriter, rr *schema.Response, acceptGzip bool) {
	var out io.Writer = w

	// set TouchedUids header
	w.Header().Set(touchedUidsHeader, strconv.FormatUint(rr.GetExtensions().GetTouchedUids(), 10))

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

	// library (graphql-transport-ws) passes the headers which are part of the INIT payload to us in the context.
	// And we are extracting the Auth JWT from those and passing them along.
	customClaims := &authorization.CustomClaims{
		StandardClaims: jwt.StandardClaims{},
	}
	header, _ := ctx.Value("Header").(json.RawMessage)

	if len(header) > 0 {
		payload := make(map[string]interface{})
		if err := json.Unmarshal(header, &payload); err != nil {
			return nil, err
		}

		name := authorization.GetHeader()
		val, ok := payload[name].(string)
		if ok {

			md := metadata.New(map[string]string{
				"authorizationJwt": val,
			})
			ctx = metadata.NewIncomingContext(ctx, md)

			customClaims, err = authorization.ExtractCustomClaims(ctx)
			if err != nil {
				return nil, err
			}
		}
	}
	// for the cases when no expiry is given in jwt or subscription doesn't have any authorization,
	// we set their expiry to zero time
	if customClaims.StandardClaims.ExpiresAt == nil {
		customClaims.StandardClaims.ExpiresAt = jwt.At(time.Time{})
	}
	req := &schema.Request{
		OperationName: operationName,
		Query:         document,
		Variables:     variableValues,
	}

	res, err := gs.graphqlHandler.poller.AddSubscriber(req, customClaims)
	if err != nil {
		return nil, err
	}

	go func() {
		// Context is cancelled when a client disconnects, so delete subscription after client
		// disconnects.
		<-ctx.Done()
		gs.graphqlHandler.poller.TerminateSubscription(res.BucketID, res.SubscriptionID)
	}()
	return res.UpdateCh, ctx.Err()
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
	if r.Method == http.MethodOptions {
		return
	}

	ctx, span := trace.StartSpan(r.Context(), "handler")
	defer span.End()

	if !gh.isValid() {
		x.Panic(errors.New("graphqlHandler not initialised"))
	}

	ctx = authorization.AttachAuthorizationJwt(ctx, r)
	ctx = x.AttachAccessJwt(ctx, r)
	// Add remote addr as peer info so that the remote address can be logged
	// inside Server.Login
	ctx = x.AttachRemoteIP(ctx, r)

	var res *schema.Response
	gqlReq, err := getRequest(ctx, r)

	if err != nil {
		res = schema.ErrorResponse(err)
	} else {
		gqlReq.Header = r.Header
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
		case "application/graphql":
			bytes, err := ioutil.ReadAll(r.Body)
			if err != nil {
				return nil, errors.Wrap(err, "Could not read GraphQL request body")
			}
			gqlReq.Query = string(bytes)
		default:
			// https://graphql.org/learn/serving-over-http/#post-request says:
			// "A standard GraphQL POST request should use the application/json
			// content type ..."
			return nil, errors.New(
				"Unrecognised Content-Type.  Please use application/json or application/graphql for GraphQL requests")
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
		// Overwrite the allowed headers after also including headers which are part of
		// forwardHeaders.
		w.Header().Set("Access-Control-Allow-Headers", schema.AllowedHeaders())

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
