/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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

package admin

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/ee/audit"
	"github.com/dgraph-io/dgraph/graphql/api"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/graphql/subscription"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/graphql-transport-ws/graphqlws"
)

type Headerkey string

const (
	touchedUidsHeader = "Graphql-TouchedUids"
)

// An IServeGraphQL can serve a GraphQL endpoint (currently only ons http)
type IServeGraphQL interface {
	// After Set is called, this IServeGraphQL serves the new resolvers for the given namespace ns.
	Set(ns uint64, schemaEpoch *uint64, resolver *resolve.RequestResolver)

	// HTTPHandler returns a http.Handler that serves GraphQL.
	HTTPHandler() http.Handler

	// ResolveWithNs processes a GQL Request using the correct resolver and returns a GQL Response
	ResolveWithNs(ctx context.Context, ns uint64, gqlReq *schema.Request) *schema.Response
}

type graphqlHandler struct {
	resolver    map[uint64]*resolve.RequestResolver
	handler     http.Handler
	poller      map[uint64]*subscription.Poller
	resolverMux sync.RWMutex // protects resolver from RW races
	pollerMux   sync.RWMutex // protects poller from RW races
}

// NewServer returns a new IServeGraphQL that can serve the given resolvers
func NewServer() IServeGraphQL {
	gh := &graphqlHandler{
		resolver: make(map[uint64]*resolve.RequestResolver),
		poller:   make(map[uint64]*subscription.Poller),
	}
	gh.handler = recoveryHandler(commonHeaders(gh.Handler()))
	return gh
}

func (gh *graphqlHandler) Set(ns uint64, schemaEpoch *uint64, resolver *resolve.RequestResolver) {
	gh.resolverMux.Lock()
	gh.resolver[ns] = resolver
	gh.resolverMux.Unlock()

	gh.pollerMux.Lock()
	gh.poller[ns] = subscription.NewPoller(schemaEpoch, resolver)
	gh.pollerMux.Unlock()
}

func (gh *graphqlHandler) HTTPHandler() http.Handler {
	return gh.handler
}

func (gh *graphqlHandler) ResolveWithNs(ctx context.Context, ns uint64,
	gqlReq *schema.Request) *schema.Response {
	gh.resolverMux.RLock()
	resolver := gh.resolver[ns]
	gh.resolverMux.RUnlock()
	return resolver.Resolve(ctx, gqlReq)
}

// write chooses between the http response writer and gzip writer
// and sends the schema response using that.
func write(w http.ResponseWriter, rr *schema.Response, acceptGzip bool) {
	var out io.Writer = w

	// set TouchedUids header
	w.Header().Set(touchedUidsHeader, strconv.FormatUint(rr.GetExtensions().GetTouchedUids(), 10))

	for key, val := range rr.Header {
		w.Header()[key] = val
	}

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

// WriteErrorResponse writes the error to the HTTP response writer in GraphQL format.
func WriteErrorResponse(w http.ResponseWriter, r *http.Request, err error) {
	write(w, schema.ErrorResponse(err), strings.Contains(r.Header.Get("Accept-Encoding"), "gzip"))
}

type graphqlSubscription struct {
	graphqlHandler *graphqlHandler
}

func (gs *graphqlSubscription) isValid(namespace uint64) error {
	gs.graphqlHandler.pollerMux.RLock()
	defer gs.graphqlHandler.pollerMux.RUnlock()
	if gs == nil {
		return errors.New("gs is nil")
	}
	if err := gs.graphqlHandler.isValid(namespace); err != nil {
		return err
	}
	if gs.graphqlHandler.poller == nil {
		return errors.New("poller is nil")
	}
	if gs.graphqlHandler.poller[namespace] == nil {
		return errors.New("poller not found")
	}
	return nil
}

func (gs *graphqlSubscription) Subscribe(
	ctx context.Context,
	document,
	operationName string,
	variableValues map[string]interface{}) (<-chan interface{}, error) {

	reqHeader := http.Header{}
	// library (graphql-transport-ws) passes the headers which are part of the INIT payload to us
	// in the context. We are extracting those headers and passing them along.
	headerPayload, _ := ctx.Value("Header").(json.RawMessage)
	if len(headerPayload) > 0 {
		headers := make(map[string]interface{})
		if err := json.Unmarshal(headerPayload, &headers); err != nil {
			return nil, err
		}

		for k, v := range headers {
			if vStr, ok := v.(string); ok {
				reqHeader.Set(k, vStr)
			}
		}
	}

	// Earlier the graphql-transport-ws library was ignoring the http headers in the request.
	// The library was relying upon the information present in the request payload. This was
	// blocker for the cloud team because the only control cloud has is over the HTTP headers.
	// This fix ensures that we are setting the request headers if not provided in the payload.
	httpHeaders, _ := ctx.Value("RequestHeader").(http.Header)
	if len(httpHeaders) > 0 {
		for k := range httpHeaders {
			if len(strings.TrimSpace(reqHeader.Get(k))) == 0 {
				reqHeader.Set(k, httpHeaders.Get(k))
			}
		}
	}

	req := &schema.Request{
		OperationName: operationName,
		Query:         document,
		Variables:     variableValues,
		Header:        reqHeader,
	}

	audit.AuditWebSockets(ctx, req)
	namespace := x.ExtractNamespaceHTTP(&http.Request{Header: reqHeader})
	glog.Infof("namespace: %d. Got GraphQL request over websocket.", namespace)
	// first load the schema, then do anything else
	if err := LazyLoadSchema(namespace); err != nil {
		return nil, err
	}
	if err := gs.isValid(namespace); err != nil {
		glog.Errorf("namespace: %d. graphqlSubscription not initialized: %s", namespace, err)
		return nil, errors.New(resolve.ErrInternal)
	}

	gs.graphqlHandler.pollerMux.RLock()
	poller := gs.graphqlHandler.poller[namespace]
	gs.graphqlHandler.pollerMux.RUnlock()

	res, err := poller.AddSubscriber(req)
	if err != nil {
		return nil, err
	}

	go func() {
		// Context is cancelled when a client disconnects, so delete subscription after client
		// disconnects.
		<-ctx.Done()
		poller.TerminateSubscription(res.BucketID, res.SubscriptionID)
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
	ctx, span := trace.StartSpan(r.Context(), "handler")
	defer span.End()

	ns, _ := strconv.ParseUint(r.Header.Get("resolver"), 10, 64)
	glog.Infof("namespace: %d. Got GraphQL request over HTTP.", ns)
	if err := gh.isValid(ns); err != nil {
		glog.Errorf("namespace: %d. graphqlHandler not initialised: %s", ns, err)
		WriteErrorResponse(w, r, errors.New(resolve.ErrInternal))
		return
	}

	gh.resolverMux.RLock()
	resolver := gh.resolver[ns]
	gh.resolverMux.RUnlock()

	addDynamicHeaders(resolver, r.Header.Get("Origin"), w)
	if r.Method == http.MethodOptions {
		// for OPTIONS, we only need to send the headers
		return
	}

	// Pass in PoorMan's auth, ACL and IP information if present.
	ctx = x.AttachAccessJwt(ctx, r)
	ctx = x.AttachRemoteIP(ctx, r)
	ctx = x.AttachAuthToken(ctx, r)
	ctx = x.AttachJWTNamespace(ctx)

	var res *schema.Response
	gqlReq, err := getRequest(r)

	if err != nil {
		WriteErrorResponse(w, r, err)
		return
	}

	if err = edgraph.ProcessPersistedQuery(ctx, gqlReq); err != nil {
		WriteErrorResponse(w, r, err)
		return
	}

	res = resolver.Resolve(ctx, gqlReq)
	write(w, res, strings.Contains(r.Header.Get("Accept-Encoding"), "gzip"))
}

func (gh *graphqlHandler) isValid(namespace uint64) error {
	gh.resolverMux.RLock()
	defer gh.resolverMux.RUnlock()
	switch {
	case gh == nil:
		return errors.New("gh is nil")
	case gh.resolver == nil:
		return errors.New("resolver is nil")
	case gh.resolver[namespace] == nil:
		return errors.New("resolver not found")
	case gh.resolver[namespace].Schema() == nil:
		return errors.New("schema is nil")
	case gh.resolver[namespace].Schema().Meta() == nil:
		return errors.New("schema meta is nil")
	}
	return nil
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

func getRequest(r *http.Request) (*schema.Request, error) {
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
		if extensions, ok := query["extensions"]; ok {
			if len(extensions) > 0 {
				d := json.NewDecoder(strings.NewReader(extensions[0]))
				d.UseNumber()
				if err := d.Decode(&gqlReq.Extensions); err != nil {
					return nil, errors.Wrap(err, "Not a valid GraphQL request body")
				}
			}
		}
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
	gqlReq.Header = r.Header

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
			}, "")

		next.ServeHTTP(w, r)
	})
}

// addDynamicHeaders adds any headers which are stored in the schema to the HTTP response.
// At present, it handles following headers:
//   - Access-Control-Allow-Headers
//   - Access-Control-Allow-Origin
func addDynamicHeaders(reqResolver *resolve.RequestResolver, origin string, w http.ResponseWriter) {
	schemaMeta := reqResolver.Schema().Meta()

	// Set allowed headers after also including headers which are part of forwardHeaders.
	w.Header().Set("Access-Control-Allow-Headers", schemaMeta.AllowedCorsHeaders())

	allowedOrigins := schemaMeta.AllowedCorsOrigins()
	if len(allowedOrigins) == 0 {
		// Since there is no allow-list to restrict, we'll allow everyone to access.
		w.Header().Set("Access-Control-Allow-Origin", "*")
	} else if allowedOrigins[origin] {
		// Let's set the respective origin address in the allow origin.
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		// otherwise, Given origin is not in the allow list, so let's remove any allowed origin.
		w.Header().Del("Access-Control-Allow-Origin")
	}
}
