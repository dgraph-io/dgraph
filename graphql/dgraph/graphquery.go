/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package dgraph

import (
	"fmt"
	"strings"

	"github.com/hypermodeinc/dgraph/v25/dql"
	"github.com/hypermodeinc/dgraph/v25/x"
)

// AsString writes query as an indented dql query string.  AsString doesn't
// validate query, and so doesn't return an error if query is 'malformed' - it might
// just write something that wouldn't parse as a Dgraph query.
func AsString(queries []*dql.GraphQuery) string {
	if len(queries) == 0 {
		return ""
	}

	var b strings.Builder
	queryName := queries[len(queries)-1].Attr
	x.Check2(b.WriteString("query "))
	addQueryVars(&b, queryName, queries[0].Args)
	x.Check2(b.WriteString("{\n"))

	numRewrittenQueries := 0
	for _, q := range queries {
		if q == nil {
			// Don't call writeQuery on a nil query
			continue
		}
		writeQuery(&b, q, "  ")
		numRewrittenQueries++
	}
	x.Check2(b.WriteString("}"))

	if numRewrittenQueries == 0 {
		// In case writeQuery has not been called on any query or all queries
		// are nil. Then, return empty string. This case needs to be considered as
		// we don't want to return query{} in this case.
		return ""
	}
	return b.String()
}

func addQueryVars(b *strings.Builder, queryName string, args map[string]string) {
	dollarFound := false
	for name, val := range args {
		if strings.HasPrefix(name, "$") {
			if !dollarFound {
				x.Check2(b.WriteString(queryName + "("))
				x.Check2(b.WriteString(name + ": " + val))
				dollarFound = true
			} else {
				x.Check2(b.WriteString(", " + name + ": " + val))
			}
		}
	}
	if dollarFound {
		x.Check2(b.WriteString(") "))
	}
}

func writeQuery(b *strings.Builder, query *dql.GraphQuery, prefix string) {
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

	if query.Func == nil && hasOrderOrPage(query) {
		x.Check2(b.WriteString(" ("))
		writeOrderAndPage(b, query, false)
		x.Check2(b.WriteRune(')'))
	}

	if len(query.Cascade) != 0 {
		if query.Cascade[0] == "__all__" {
			x.Check2(b.WriteString(" @cascade"))
		} else {
			x.Check2(b.WriteString(" @cascade("))
			x.Check2(b.WriteString(strings.Join(query.Cascade, ", ")))
			x.Check2(b.WriteRune(')'))
		}
	}

	switch {
	case len(query.Children) > 0:
		prefixAdd := ""
		if query.Attr != "" {
			x.Check2(b.WriteString(" {\n"))
			prefixAdd = "  "
		}
		for _, c := range query.Children {
			writeQuery(b, c, prefix+prefixAdd)
		}
		if query.Attr != "" {
			x.Check2(b.WriteString(prefix))
			x.Check2(b.WriteString("}\n"))
		}
	case query.Var != "" || query.Alias != "" || query.Attr != "":
		x.Check2(b.WriteString("\n"))
	}
}

func writeUIDFunc(b *strings.Builder, uids []uint64, args []dql.Arg) {
	x.Check2(b.WriteString("uid("))
	if len(uids) > 0 {
		// uid function with uint64 - uid(0x123, 0x456, ...)
		for i, uid := range uids {
			if i != 0 {
				x.Check2(b.WriteString(", "))
			}
			x.Check2(b.WriteString(fmt.Sprintf("%#x", uid)))
		}
	} else {
		// uid function with a Dgraph query variable - uid(Post1)
		for i, arg := range args {
			if i != 0 {
				x.Check2(b.WriteString(", "))
			}
			x.Check2(b.WriteString(arg.Value))
		}
	}
	x.Check2(b.WriteString(")"))
}

// writeRoot writes the root function as well as any ordering and paging
// specified in q.
//
// Only uid(0x123, 0x124), type(...) and eq(Type.Predicate, ...) functions are supported at root.
// Multiple arguments for `eq` filter will be required in case of resolving `entities` query.
func writeRoot(b *strings.Builder, q *dql.GraphQuery) {
	if q.Func == nil {
		return
	}

	switch {
	// TODO: Instead of the hard-coded strings "uid", "type", etc., use the
	// pre-defined constants in dql/parser.go such as dql.uidFunc, dql.typFunc,
	// etc. This of course will require that we make these constants public.
	case q.Func.Name == "uid":
		x.Check2(b.WriteString("(func: "))
		writeUIDFunc(b, q.Func.UID, q.Func.Args)
	case q.Func.Name == "type" && len(q.Func.Args) == 1:
		x.Check2(b.WriteString(fmt.Sprintf("(func: type(%s)", q.Func.Args[0].Value)))
	case q.Func.Name == "eq":
		x.Check2(b.WriteString("(func: eq("))
		writeFilterArguments(b, q.Func.Args)
		x.Check2(b.WriteRune(')'))
	case q.Func.Name == "similar_to":
		x.Check2(b.WriteString("(func: similar_to("))
		writeFilterArguments(b, q.Func.Args)
		x.Check2(b.WriteRune(')'))
	}
	writeOrderAndPage(b, q, true)
	x.Check2(b.WriteRune(')'))
}

func writeFilterArguments(b *strings.Builder, args []dql.Arg) {
	for i, arg := range args {
		if i != 0 {
			x.Check2(b.WriteString(", "))
		}
		x.Check2(b.WriteString(arg.Value))
	}
}

func writeFilterFunction(b *strings.Builder, f *dql.Function) {
	if f == nil {
		return
	}

	switch {
	case f.Name == "uid":
		writeUIDFunc(b, f.UID, f.Args)
	default:
		x.Check2(b.WriteString(fmt.Sprintf("%s(", f.Name)))
		writeFilterArguments(b, f.Args)
		x.Check2(b.WriteRune(')'))
	}
}

func writeFilter(b *strings.Builder, ft *dql.FilterTree) {
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

func hasOrderOrPage(q *dql.GraphQuery) bool {
	_, hasFirst := q.Args["first"]
	_, hasOffset := q.Args["offset"]
	return len(q.Order) > 0 || hasFirst || hasOffset
}

func writeOrderAndPage(b *strings.Builder, query *dql.GraphQuery, root bool) {
	var wroteOrder, wroteFirst bool

	for _, ord := range query.Order {
		if root || wroteOrder {
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
