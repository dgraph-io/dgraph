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
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

func resolveHealth(ctx context.Context, q schema.Query) *resolve.Resolved {
	glog.Info("Got health request")

	var buf bytes.Buffer
	x.Check2(buf.WriteString(`"health":`))

	var resp *api.Response
	var err error
	if resp, err = (&edgraph.Server{}).Health(ctx, true); err != nil {
		err = errors.Errorf("%s: %s", x.Error, err.Error())
		x.Check2(buf.Write([]byte(` null `)))
	} else {
		x.Check2(buf.Write(resp.GetJson()))
	}

	return &resolve.Resolved{
		Data: buf.Bytes(),
		Err:  err,
	}
}
