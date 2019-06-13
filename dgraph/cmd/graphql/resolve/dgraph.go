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

func executeQuery(query *gql.GraphQuery, dgraphClient *dgo.Dgraph) ([]byte, error) {

	q := asString(query)
	glog.V(2).Infof("Executing Dgraph query: \n%s\n", q)

	ctx := context.Background()
	resp, err := dgraphClient.NewTxn().Query(ctx, q)
	if err != nil {
		// should I be inspecting this to work out if it's a bug,
		// or a transient error?
		return nil, errors.Wrap(err, "while querying Dgraph")
	}

	return resp.Json, nil
}

// TODO: extract the below into gql package??
//
// There's not much of GraphQuery supported so far,
// so just simple writes of what is supported.
func asString(query *gql.GraphQuery) string {
	var b strings.Builder

	b.WriteString("query {\n")
	writeQuery(query, &b, "  ")
	b.WriteString("}")

	return b.String()
}

func writeQuery(query *gql.GraphQuery, b *strings.Builder, prefix string) {
	b.WriteString(prefix)
	if query.Alias != "" {
		b.WriteString(query.Alias)
		b.WriteString(" : ")
	}
	b.WriteString(query.Attr)

	writeFunction(query.Func, b)
	writeFilter(query.Filter, b)

	if len(query.Children) > 0 {
		b.WriteString(" { \n")
		for _, c := range query.Children {
			writeQuery(c, b, prefix+"  ")
		}
		b.WriteString(prefix)
		b.WriteString("}")
	}
	b.WriteString("\n")
}

func writeFunction(f *gql.Function, b *strings.Builder) {
	if f != nil {
		b.WriteString(fmt.Sprintf("(func: %s(0x%x)) ", f.Name, f.UID[0]))
		// there's only uid(...) functions so far
	}
}

func writeFilterFunction(f *gql.Function, b *strings.Builder) {
	if f != nil {
		b.WriteString(fmt.Sprintf("%s(%s) ", f.Name, f.Args[0].Value))
	}
}

func writeFilter(f *gql.FilterTree, b *strings.Builder) {
	if f == nil {
		return
	}

	if f.Func.Name == "type" {
		b.WriteString(fmt.Sprintf("@filter(type(%s)) ", f.Func.Args[0].Value))
	} else {
		b.WriteString("@filter(")
		writeFilterFunction(f.Func, b)
		b.WriteString(")")
	}
}
