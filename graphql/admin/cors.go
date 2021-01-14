/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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
	"context"
	"net/url"

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
)

// getOrigins retrieve the origins from the arguments and returns the retrieved origins if
// the provided origins are valid.
func getOrigins(m schema.Mutation) ([]string, error) {
	out := []string{}
	for _, origin := range m.ArgValue("origins").([]interface{}) {
		castedOrigin := origin.(string)
		// Validate the origin.
		_, err := url.Parse(castedOrigin)
		if err != nil {
			return nil, err
		}
		out = append(out, castedOrigin)
	}
	return out, nil
}

// resolveUpdateCors update the cors details.
func resolveReplaceAllowedCORSOrigins(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	origins, err := getOrigins(m)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}
	// Aleast one origin is required to add allowlist. Since, no origin is provided, so we'll
	// all origin to access dgraph.
	if len(origins) == 0 {
		origins = append(origins, "*")
	}
	if err = edgraph.AddCorsOrigins(ctx, origins); err != nil {
		return resolve.EmptyResult(m, err), false
	}
	return &resolve.Resolved{
		Data: map[string]interface{}{
			m.Name(): map[string]interface{}{
				"acceptedOrigins": arrayToInterface(origins),
			},
		},
		Field: m,
		Err:   nil,
	}, true
}

// resolveGetCors retrieves cors details from the database.
func resolveGetCors(ctx context.Context, q schema.Query) *resolve.Resolved {
	_, origins, err := edgraph.GetCorsOrigins(ctx)
	if err != nil {
		return resolve.EmptyResult(q, err)
	}
	return &resolve.Resolved{
		Data: map[string]interface{}{
			q.Name(): map[string]interface{}{
				"acceptedOrigins": arrayToInterface(origins),
			},
		},
		Field: q,
	}
}

// arrayToInterface convers array string to array interface
func arrayToInterface(in []string) []interface{} {
	out := make([]interface{}, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}
