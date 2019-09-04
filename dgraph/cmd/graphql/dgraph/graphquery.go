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

func asString(query *gql.GraphQuery) string {
	if query == nil {
		return ""
	}

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
		b.WriteString(" {\n")
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
		b.WriteString(fmt.Sprintf("(func: %s(0x%x))", f.Name, f.UID[0]))
		// there's only uid(...) functions so far
	}
}

func writeFilterFunction(b *strings.Builder, f *gql.Function) {
	if f != nil {
		b.WriteString(fmt.Sprintf("%s(%s)", f.Name, f.Args[0].Value))
	}
}

func writeFilter(b *strings.Builder, f *gql.FilterTree) {
	if f == nil {
		return
	}

	if f.Func.Name == "type" {
		b.WriteString(fmt.Sprintf(" @filter(type(%s))", f.Func.Args[0].Value))
	} else {
		b.WriteString(" @filter(")
		writeFilterFunction(b, f.Func)
		b.WriteString(")")
	}
}
