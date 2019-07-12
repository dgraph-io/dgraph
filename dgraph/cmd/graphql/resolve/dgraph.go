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

package resolve

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/pkg/errors"
)

func executeQuery(ctx context.Context, query *gql.GraphQuery, dgraphClient *dgo.Dgraph) ([]byte, error) {

	q := asString(query)
	if glog.V(3) {
		glog.Infof("Executing Dgraph query: \n%s\n", q)
	}

	resp, err := dgraphClient.NewTxn().Query(ctx, q)
	if err != nil {
		// should I be inspecting this to work out if it's a bug,
		// or a transient error?
		return nil, errors.Wrap(err, "while querying Dgraph")
	}

	return resp.Json, nil
}

// TODO: once there's more below, extract out into gql package??
//
// There's not much of GraphQuery supported so far,
// so just simple writes of what is supported.
func asString(query *gql.GraphQuery) string {
	var b strings.Builder

	b.WriteString("query {\n")
	writeQuery(&b, query, "  ")
	b.WriteString("}")

	return b.String()
}

func writeQuery(b *strings.Builder, query *gql.GraphQuery, prefix string) {
	b.WriteString(prefix)
	if query.Alias != "" {
		b.WriteString(query.Alias)
		b.WriteString(" : ")
	}
	b.WriteString(query.Attr)

	writeFunction(b, query.Func)
	writeFilter(b, query.Filter)

	if len(query.Children) > 0 {
		b.WriteString(" { \n")
		for _, c := range query.Children {
			writeQuery(b, c, prefix+"  ")
		}
		b.WriteString(prefix)
		b.WriteString("}")
	}
	b.WriteString("\n")
}

func writeFunction(b *strings.Builder, f *gql.Function) {
	if f != nil {
		b.WriteString(fmt.Sprintf("(func: %s(0x%x)) ", f.Name, f.UID[0]))
		// there's only uid(...) functions so far
	}
}

func writeFilterFunction(b *strings.Builder, f *gql.Function) {
	if f != nil {
		b.WriteString(fmt.Sprintf("%s(%s) ", f.Name, f.Args[0].Value))
	}
}

func writeFilter(b *strings.Builder, f *gql.FilterTree) {
	if f == nil {
		return
	}

	if f.Func.Name == "type" {
		b.WriteString(fmt.Sprintf("@filter(type(%s)) ", f.Func.Args[0].Value))
	} else {
		b.WriteString("@filter(")
		writeFilterFunction(b, f.Func)
		b.WriteString(")")
	}
}
