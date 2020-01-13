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

package api

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// Various functions for getting and setting context, etc. used
// throughout the GraphQL API.

type apiKey string

const (
	requestID    = apiKey("requestID")
	nilRequestID = "a1111111-b222-c333-d444-e55555555555"

	gqlQuery    = apiKey("gqlQuery")
	nilGQLQuery = "aGraphQLQuery {}"
)

// WithRequestID adds a HTTP middleware handler that sets a UUID as request ID
// into the handler chain.
func WithRequestID(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := uuid.New().String()
		h.ServeHTTP(w, r.WithContext(
			context.WithValue(r.Context(), requestID, reqID)))
	})
}

// WithQueryString returns a new context where QueryString() returns query.
func WithQueryString(ctx context.Context, query string) context.Context {
	return context.WithValue(ctx, gqlQuery, query)
}

// RequestID gets the request ID from a context.  If no request ID is set,
// it returns "a1111111-b222-c333-d444-e55555555555".
func RequestID(ctx context.Context) string {
	return stringFromContext(ctx, requestID, nilRequestID)
}

// QueryString gets the GraphQL query from a context.  If no query is set,
// it returns "aGraphQLQuery {}"
func QueryString(ctx context.Context) string {
	return stringFromContext(ctx, gqlQuery, nilGQLQuery)
}

func stringFromContext(ctx context.Context, key apiKey, def string) string {
	if val := ctx.Value(key); val != nil {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return def
}
