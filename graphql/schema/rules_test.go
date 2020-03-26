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

package schema

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

func TestParseGraphqlMethod(t *testing.T) {
	method, err := parseGraphqlMethod("hello(sdf:$sd)")
	require.NoError(t, err)
	require.Equal(t, method.name, "hello")
	require.Equal(t, method.args, map[string]string{"sdf": "$sd"})

	method, err = parseGraphqlMethod("hello(sdf:$sd,sdf2 :$sd3)")
	require.NoError(t, err)
	require.Equal(t, method.name, "hello")
	require.Equal(t, method.args, map[string]string{"sdf": "$sd",
		"sdf2": "$sd3"})

	_, err = parseGraphqlMethod("hello")
	require.Error(t, err)

	_, err = parseGraphqlMethod("hello(")
	require.Error(t, err)

	_, err = parseGraphqlMethod("hello(:")
	require.Error(t, err)

	_, err = parseGraphqlMethod("hello(asd:")
	require.Error(t, err)

	_, err = parseGraphqlMethod("hello(asd:)")
	require.Error(t, err)

	_, err = parseGraphqlMethod("hello(asd:,as:$asd)")
	require.Error(t, err)

	_, err = parseGraphqlMethod("hello(asd:as,)")
	require.Error(t, err)

	_, err = parseGraphqlMethod("hello(asd:as,)")
	require.Error(t, err)

	_, err = parseGraphqlMethod("hello(asd:$)")
	require.Error(t, err)
}

func TestSumma(t *testing.T) {
	doc, gqlErr := parser.ParseQuery(&ast.Source{Input: `query { getUser(id: {hello: $yo}, da:"sdf") }`})
	require.Nil(t, gqlErr)
	field := doc.Operations[0].SelectionSet[0].(*ast.Field)
	fmt.Println(field.Name)
	for _, arg := range field.Arguments {
		fmt.Println(arg.Value)
	}
}
