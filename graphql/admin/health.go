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

package admin

import (
	"bytes"
	"context"

	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

type healthResolver struct {
}

func (hr *healthResolver) Rewrite(ctx context.Context, q schema.Query) (*gql.GraphQuery, error) {
	return nil, nil
}

func (hr *healthResolver) Query(ctx context.Context, query *gql.GraphQuery) ([]byte, error) {
	var err error

	var resp *api.Response
	var respErr error
	if resp, respErr = (&edgraph.Server{}).Health(ctx, true); respErr != nil {
		err = errors.Errorf("%s: %s", x.Error, respErr.Error())
	}
	if resp == nil {
		err = errors.Errorf("%s: %s", x.ErrorNoData, "No state information available.")
	}

	var buf bytes.Buffer
	x.Check2(buf.WriteString(`{ "health":`))
	x.Check2(buf.Write(resp.Json))
	x.Check2(buf.WriteString(`}`))

	return buf.Bytes(), err
}
