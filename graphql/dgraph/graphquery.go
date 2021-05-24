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
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
)

// AsString writes query as an indented dql query string.  AsString doesn't
// validate query, and so doesn't return an error if query is 'malformed' - it might
// just write something that wouldn't parse as a Dgraph query.
func AsString(queries []*gql.GraphQuery) string {
	if queries == nil {
		return ""
	}

	var b strings.Builder
	x.Check2(b.WriteString("query {\n"))
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

func writeQuery(b *strings.Builder, query *gql.GraphQuery, prefix string) {
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

	if query.IsCount {
		x.Check2(b.WriteString(fmt.Sprintf("count(%s)", query.Attr)))
	} else if query.Attr != "val" {
		x.Check2(b.WriteString(query.Attr))
	} else if isAggregateFn(query.Func) {
		x.Check2(b.WriteString("sum(val("))
		writeNeedVar(b, query)
		x.Check2(b.WriteRune(')'))
	} else {
		x.Check2(b.WriteString("val("))
		writeNeedVar(b, query)
		x.Check2(b.WriteRune(')'))
	}

	if query.Func != nil {
		writeRoot(b, query)
		x.Check2(b.WriteRune(')'))
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

	if query.IsGroupby {
		x.Check2(b.WriteString(" @groupby("))
		writeGroupByAttributes(b, query.GroupbyAttrs)
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

// writeNeedVar writes the NeedsVar of the query. For eg :-
// `userFollowerCount as sum(val(followers))` has `followers`
// as NeedsVar.
func writeNeedVar(b *strings.Builder, query *gql.GraphQuery) {
	for i, v := range query.NeedsVar {
		if i != 0 {
			x.Check2(b.WriteString(", "))
		}
		x.Check2(b.WriteString(v.Name))
	}
}

func isAggregateFn(f *gql.Function) bool {
	if f == nil {
		return false
	}
	switch f.Name {
	case "min", "max", "avg", "sum":
		return true
	}
	return false
}

func writeGroupByAttributes(b *strings.Builder, attrList []gql.GroupByAttr) {
	for i, attr := range attrList {
		if i != 0 {
			x.Check2(b.WriteString(", "))
		}
		if attr.Alias != "" {
			x.Check2(b.WriteString(attr.Alias))
			x.Check2(b.WriteString(" : "))
		}
		x.Check2(b.WriteString(attr.Attr))
	}
}

func writeUIDFunc(b *strings.Builder, uids []uint64, args []gql.Arg, needVar []gql.VarContext) {
	x.Check2(b.WriteString("uid("))
	if len(uids) > 0 {
		// uid function with uint64 - uid(0x123, 0x456, ...)
		for i, uid := range uids {
			if i != 0 {
				x.Check2(b.WriteString(", "))
			}
			x.Check2(b.WriteString(fmt.Sprintf("%#x", uid)))
		}
	} else if len(args) > 0 {
		// uid function with a Dgraph query variable - uid(Post1)
		for i, arg := range args {
			if i != 0 {
				x.Check2(b.WriteString(", "))
			}
			x.Check2(b.WriteString(arg.Value))
		}
	} else {
		for i, v := range needVar {
			if i != 0 {
				x.Check2(b.WriteString(", "))
			}
			x.Check2(b.WriteString(v.Name))
		}
	}
	x.Check2(b.WriteString(")"))
}

// writeRoot writes the root function as well as any ordering and paging
// specified in q.
//
// Only uid(0x123, 0x124), type(...) and eq(Type.Predicate, ...) functions are supported at root.
// Multiple arguments for `eq` filter will be required in case of resolving `entities` query.
func writeRoot(b *strings.Builder, q *gql.GraphQuery) {
	if q.Func == nil {
		return
	}

	switch {
	case q.Func.Name == "has":
		x.Check2(b.WriteString(fmt.Sprintf("(func: has(%s)", q.Func.Attr)))
	case q.Func.Name == "uid":
		x.Check2(b.WriteString("(func: "))
		writeUIDFunc(b, q.Func.UID, q.Func.Args, q.Func.NeedsVar)
	case q.Func.Name == "type" && len(q.Func.Args) == 1:
		x.Check2(b.WriteString(fmt.Sprintf("(func: type(%s)", q.Func.Args[0].Value)))
	case q.Func.Name == "eq":
		x.Check2(b.WriteString("(func: eq("))
		writeFilterArguments(b, q.Func)
		x.Check2(b.WriteRune(')'))
	}
	writeOrderAndPage(b, q, true)
}

// writeFilterArguments writes the filter arguments. If the filter
// is constructed in graphql query rewriting then `Attr` is an empty
// string since we add Attr in the argument itself.
func writeFilterArguments(b *strings.Builder, q *gql.Function) {
	if q.Attr != "" {
		x.Check2(b.WriteString(q.Attr))
	}

	for i, arg := range q.Args {
		if i != 0 || q.Attr != "" {
			x.Check2(b.WriteString(", "))
		}
		if q.Attr != "" {
			// quote the arguments since this is the case of
			// @custom DQL string.
			arg.Value = schema.MaybeQuoteArg(q.Name, arg.Value)
		}
		x.Check2(b.WriteString(arg.Value))
	}
}

func writeFilterFunction(b *strings.Builder, f *gql.Function) {
	if f == nil {
		return
	}

	switch {
	case f.Name == "uid":
		writeUIDFunc(b, f.UID, f.Args, f.NeedsVar)
	default:
		x.Check2(b.WriteString(fmt.Sprintf("%s(", f.Name)))
		writeFilterArguments(b, f)
		x.Check2(b.WriteRune(')'))
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

func IsValueVar(attr string, q *gql.GraphQuery) bool {
	for _, vars := range q.NeedsVar {
		if attr == vars.Name && vars.Typ == 2 {
			return true
		}
	}
	return false
}

func writeOrderAndPage(b *strings.Builder, query *gql.GraphQuery, root bool) {
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
		if IsValueVar(ord.Attr, query) {
			x.Check2(b.WriteString("val("))
			x.Check2(b.WriteString(ord.Attr))
			x.Check2(b.WriteRune(')'))
		} else {
			x.Check2(b.WriteString(ord.Attr))
		}
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
