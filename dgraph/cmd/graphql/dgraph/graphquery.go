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
)

// asString writes query as an indented GraphQL+- query string.  AsString doesn't
// validate query, and so doesn't return an error if query is 'malformed' - it might
// just write something that wouldn't parse as a Dgraph query.
func asString(query *gql.GraphQuery) string {
	var b strings.Builder

	b.WriteString("query {\n")
	writeQuery(&b, query, "  ", true)
	b.WriteString("}")

	return b.String()
}

func writeQuery(b *strings.Builder, query *gql.GraphQuery, prefix string, root bool) {
	b.WriteString(prefix)
	if query.Alias != "" {
		b.WriteString(query.Alias)
		b.WriteString(" : ")
	}
	b.WriteString(query.Attr)

	if root {
		writeRoot(b, query)
	}

	if query.Filter != nil {
		b.WriteString(" @filter(")
		writeFilter(b, query.Filter)
		b.WriteRune(')')
	}

	if !root && hasOrderOrPage(query) {
		b.WriteString(" (")
		writeOrderAndPage(b, query, false)
		b.WriteRune(')')
	}

	if len(query.Children) > 0 {
		b.WriteString(" {\n")
		for _, c := range query.Children {
			writeQuery(b, c, prefix+"  ", false)
		}
		b.WriteString(prefix)
		b.WriteString("}")
	}
	b.WriteString("\n")
}

// writeRoot writes the root function as well as any ordering and paging
// specified in q.
//
// Only uid(0x123) and type(...) functions are supported at root.
func writeRoot(b *strings.Builder, q *gql.GraphQuery) {
	if q.Func == nil {
		return
	}

	if q.Func.Name == "uid" && len(q.Func.UID) == 1 {
		b.WriteString(fmt.Sprintf("(func: uid(%#x))", q.Func.UID[0]))
	} else if q.Func.Name == "type" && len(q.Func.Args) == 1 {
		b.WriteString(fmt.Sprintf("(func: type(%s)", q.Func.Args[0].Value))
		writeOrderAndPage(b, q, true)
		b.WriteRune(')')
	}
}

func writeFilterFunction(b *strings.Builder, f *gql.Function) {
	if f == nil {
		return
	}

	if len(f.Args) == 1 {
		b.WriteString(fmt.Sprintf("%s(%s)", f.Name, f.Args[0].Value))
	} else if len(f.Args) == 2 {
		b.WriteString(fmt.Sprintf("%s(%s, %s)", f.Name, f.Args[0].Value, f.Args[1].Value))
	}
}

func writeFilter(b *strings.Builder, ft *gql.FilterTree) {
	if ft == nil {
		return
	}

	switch ft.Op {
	case "and", "or":
		b.WriteRune('(')
		for i, child := range ft.Child {
			if i > 0 && i <= len(ft.Child)-1 {
				b.WriteString(fmt.Sprintf(" %s ", strings.ToUpper(ft.Op)))
			}
			writeFilter(b, child)
		}
		b.WriteRune(')')
	case "not":
		if len(ft.Child) > 0 {
			b.WriteString("NOT (")
			writeFilter(b, ft.Child[0])
			b.WriteRune(')')
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
			b.WriteString(", ")
		}
		if ord.Desc {
			b.WriteString("orderdesc: ")
		} else {
			b.WriteString("orderasc: ")
		}
		b.WriteString(ord.Attr)
		wroteOrder = true
	}

	if first, ok := query.Args["first"]; ok {
		if root || wroteOrder {
			b.WriteString(", ")
		}
		b.WriteString("first: ")
		b.WriteString(first)
		wroteFirst = true
	}

	if offset, ok := query.Args["offset"]; ok {
		if root || wroteOrder || wroteFirst {
			b.WriteString(", ")
		}
		b.WriteString("offset: ")
		b.WriteString(offset)
	}
}
