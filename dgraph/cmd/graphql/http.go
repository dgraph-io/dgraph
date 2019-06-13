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
	"errors"
	"mime"
	"net/http"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/resolve"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
)

type graphqlHTTPHandler struct {
	dgraphClient *dgo.Dgraph
	schema       *ast.Schema
}

// ServeHTTP handles GraphQL queries and mutations that get resolved
// via GraphQL->Dgraph->GraphQL.  It writes a valid GraphQL JSON response
// to w.
func (gh *graphqlHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if gh == nil || gh.schema == nil || gh.dgraphClient == nil {
		glog.Error(errors.New("Handler not initialised"))

		b, err := json.Marshal(schema.ErrorResponsef("Server not correctly initialised"))
		if err != nil {
			glog.Error(err)
			return
		}
		_, err = w.Write(b)
		if err != nil {
			glog.Error(err)
			return
		}
	}

	rh := gh.resolverForRequest(r)
	res := rh.Resolve()
	_, err := res.WriteTo(w)
	if err != nil {
		glog.Error(err)
	}
}

func (gh *graphqlHTTPHandler) resolverForRequest(r *http.Request) (rr *resolve.RequestResolver) {
	rr = resolve.New(schema.AsSchema(gh.schema), gh.dgraphClient)

	switch r.Method {
	case http.MethodGet:
		// TODO: fill gqlReq in from parameters
	case http.MethodPost:
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			rr.WithErrors(gqlerror.Errorf("Unable to parse media type: %s", err))
			return
		}

		switch mediaType {
		case "application/json":
			if err = json.NewDecoder(r.Body).Decode(&rr.GqlReq); err != nil {
				rr.WithErrors(
					gqlerror.Errorf("Not a valid GraphQL request body: %s", err))
				return
			}
		default:
			// https://graphql.org/learn/serving-over-http/#post-request says:
			// "A standard GraphQL POST request should use the application/json
			// content type ..."
			rr.WithErrors(gqlerror.Errorf(
				"Unrecognised Content-Type.  Please use application/json for GraphQL requests"))
			return
		}
	default:
		rr.WithErrors(gqlerror.Errorf(
			"Unrecognised request method.  Please use GET or POST for GraphQL requests"))
		return
	}
	return
}
