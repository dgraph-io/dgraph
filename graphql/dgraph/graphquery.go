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

package dgraph

import (
	"fmt"
	"strings"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/x"
)

// AsString writes query as an indented GraphQL+- query string.  AsString doesn't
// validate query, and so doesn't return an error if query is 'malformed' - it might
// just write something that wouldn't parse as a Dgraph query.
func AsString(query *gql.GraphQuery) string {
	if query == nil {
		return ""
	}

	var b strings.Builder
	x.Check2(b.WriteString("query {\n"))
	writeQuery(&b, query, "  ", true)
	x.Check2(b.WriteString("}"))

	return b.String()
}

func writeQuery(b *strings.Builder, query *gql.GraphQuery, prefix string, root bool) {
	if query.Var != "" || query.Alias != "" || query.Attr != "" {
		x.Check2(b.WriteString(prefix))
	}
	if query.Var != "" {
		x.Check2(b.WriteString(fmt.Sprintf("%s as ", query.Var)))
	}
	if query.Alias != "" {
		x.Check2(b.WriteString(query.Alias))
		x.Check2(b.WriteString(" : "))
	}
	x.Check2(b.WriteString(query.Attr))

	if query.Func != nil {
		writeRoot(b, query)
	}

	if query.Filter != nil {
		x.Check2(b.WriteString(" @filter("))
		writeFilter(b, query.Filter)
		x.Check2(b.WriteRune(')'))
	}

	if !root && hasOrderOrPage(query) {
		x.Check2(b.WriteString(" ("))
		writeOrderAndPage(b, query, false)
		x.Check2(b.WriteRune(')'))
	}

	switch {
	case len(query.Children) > 0:
		prefixAdd := ""
		if query.Attr != "" {
			x.Check2(b.WriteString(" {\n"))
			prefixAdd = "  "
		}
		for _, c := range query.Children {
			writeQuery(b, c, prefix+prefixAdd, false)
		}
		if query.Attr != "" {
			x.Check2(b.WriteString(prefix))
			x.Check2(b.WriteString("}\n"))
		}
	case query.Var != "" || query.Alias != "" || query.Attr != "":
		x.Check2(b.WriteString("\n"))
	}
}

func writeUidFunc(b *strings.Builder, uids []uint64) {
	x.Check2(b.WriteString("uid("))
	for i, uid := range uids {
		if i != 0 {
			x.Check2(b.WriteString(", "))
		}
		x.Check2(b.WriteString(fmt.Sprintf("%#x", uid)))
	}
	x.Check2(b.WriteString(")"))
}

// writeRoot writes the root function as well as any ordering and paging
// specified in q.
//
// Only uid(0x123, 0x124) and type(...) functions are supported at root.
func writeRoot(b *strings.Builder, q *gql.GraphQuery) {
	if q.Func == nil {
		return
	}

	switch {
	case q.Func.Name == "uid":
		x.Check2(b.WriteString("(func: "))
		writeUidFunc(b, q.Func.UID)
	case q.Func.Name == "type" && len(q.Func.Args) == 1:
		x.Check2(b.WriteString(fmt.Sprintf("(func: type(%s)", q.Func.Args[0].Value)))
	case q.Func.Name == "eq" && len(q.Func.Args) == 2:
		x.Check2(b.WriteString(fmt.Sprintf("(func: eq(%s, %s)", q.Func.Args[0].Value,
			q.Func.Args[1].Value)))
	}
	writeOrderAndPage(b, q, true)
	x.Check2(b.WriteRune(')'))
}

func writeFilterFunction(b *strings.Builder, f *gql.Function) {
	if f == nil {
		return
	}

	switch {
	case f.Name == "uid":
		writeUidFunc(b, f.UID)
	case len(f.Args) == 1:
		x.Check2(b.WriteString(fmt.Sprintf("%s(%s)", f.Name, f.Args[0].Value)))
	case len(f.Args) == 2:
		x.Check2(b.WriteString(fmt.Sprintf("%s(%s, %s)", f.Name, f.Args[0].Value, f.Args[1].Value)))
	}
}

func writeFilter(b *strings.Builder, ft *gql.FilterTree) {
	if ft == nil {
		return
	}

	switch ft.Op {
	case "and", "or":
		x.Check2(b.WriteRune('('))
		for i, child := range ft.Child {
			if i > 0 && i <= len(ft.Child)-1 {
				x.Check2(b.WriteString(fmt.Sprintf(" %s ", strings.ToUpper(ft.Op))))
			}
			writeFilter(b, child)
		}
		x.Check2(b.WriteRune(')'))
	case "not":
		if len(ft.Child) > 0 {
			x.Check2(b.WriteString("NOT ("))
			writeFilter(b, ft.Child[0])
			x.Check2(b.WriteRune(')'))
		}
	default:
		writeFilterFunction(b, ft.Func)
	}
}

func hasOrderOrPage(q *gql.GraphQuery) bool {
	_, hasFirst := q.Args["first"]
	_, hasOffset := q.Args["offset"]
	return len(q.Order) > 0 || hasFirst || hasOffset
}

func writeOrderAndPage(b *strings.Builder, query *gql.GraphQuery, root bool) {
	var wroteOrder, wroteFirst bool

	for _, ord := range query.Order {
		if root {
			x.Check2(b.WriteString(", "))
		}
		if ord.Desc {
			x.Check2(b.WriteString("orderdesc: "))
		} else {
			x.Check2(b.WriteString("orderasc: "))
		}
		x.Check2(b.WriteString(ord.Attr))
		wroteOrder = true
	}

	if first, ok := query.Args["first"]; ok {
		if root || wroteOrder {
			x.Check2(b.WriteString(", "))
		}
		x.Check2(b.WriteString("first: "))
		x.Check2(b.WriteString(first))
		wroteFirst = true
	}

	if offset, ok := query.Args["offset"]; ok {
		if root || wroteOrder || wroteFirst {
			x.Check2(b.WriteString(", "))
		}
		x.Check2(b.WriteString("offset: "))
		x.Check2(b.WriteString(offset))
	}
}
