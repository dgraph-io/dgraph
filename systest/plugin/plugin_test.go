//go:build integration || upgrade

/*
 * Copyright 2017-2023 Dgraph Labs, Inc. and Contributors
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

package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v230/protos/api"
	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/x"
)

type testCase struct {
	query      string
	wantResult string
}

var testInp = []struct {
	initialSchema string
	setJSON       string
	cases         []testCase
}{
	{
		"word: string @index(anagram) .",
		`[
			{ "word": "airmen" },
			{ "word": "marine" },
			{ "word": "beat" },
			{ "word": "beta" },
			{ "word": "race" },
			{ "word": "care" }
		]`,
		[]testCase{
			{`
				{ q(func: allof(word, anagram, "remain")) {
					word
				}}`, `
				{ "q": [
					{ "word": "airmen" },
					{ "word": "marine" }
				]}`,
			},
			{`
				{ q(func: allof(word, anagram, "acre")) {
					word
				}}`, `
				{ "q": [
					{ "word": "race" },
					{ "word": "care" }
				]}`,
			},
			{`
				{ q(func: allof(word, anagram, "beta")) {
					word
				}}`, `
				{ "q": [
					{ "word": "beta" },
					{ "word": "beat" }
				]}`,
			},
		},
	},
	{
		"word: string @index(anagram) @lang .",
		`[
			{ "word@en": "airmen", "word@fr": "no match" },
			{ "word@en": "no match", "word@fr": "marine" }
		]`,
		[]testCase{
			{`
				{ q(func: allof(word@en, anagram, "remain")) {
					word: word@en
				}}`, `
				{ "q": [
					{ "word": "airmen" }
				]}`,
			},
		},
	},
	{
		"ip: string @index(cidr) .",
		`[
			{ "ip": "100.55.22.11/32" },
			{ "ip": "100.33.81.19/32" },
			{ "ip": "100.49.21.25/32" },
			{ "ip": "101.0.0.5/32" },
			{ "ip": "100.176.2.1/32" }
		]`,
		[]testCase{
			{`
				{ q(func: allof(ip, cidr, "100.48.0.0/12")) {
					ip
				}}`, `
				{ "q": [
					{ "ip": "100.55.22.11/32" },
					{ "ip": "100.49.21.25/32" }
				]}`,
			},
			{`
				{ q(func: allof(ip, cidr, "100.32.0.0/11")) {
					ip
				}}`, `
				{ "q": [
					{ "ip": "100.55.22.11/32" },
					{ "ip": "100.33.81.19/32" },
					{ "ip": "100.49.21.25/32" }
				]}`,
			},
			{`
				{ q(func: allof(ip, cidr, "100.0.0.0/8")) {
					ip
				}}`, `
				{ "q": [
					{ "ip": "100.55.22.11/32" },
					{ "ip": "100.33.81.19/32" },
					{ "ip": "100.49.21.25/32" },
					{ "ip": "100.176.2.1/32" }
				]}`,
			},
			{`
				{ q(func: allof(ip, cidr, "101.0.0.0/8")) {
					ip
				}}`, `
				{ "q": [
					{ "ip": "101.0.0.5/32" }
				]}`,
			},
		},
	},
	{
		"name: string @index(rune) .",
		`[
			{ "name": "Adam" },
			{ "name": "Aaron" },
			{ "name": "Amy" },
			{ "name": "Ronald" }
		]`,
		[]testCase{
			{`
				{ q(func: allof(name, rune, "An")) {
					name
				}}`, `
				{ "q": [
					{ "name": "Aaron" }
				]}`,
			},
			{`
				{ q(func: allof(name, rune, "Am")) {
					name
				}}`, `
				{ "q": [
					{ "name": "Amy" },
					{ "name": "Adam" }
				]}`,
			},
			{`
				{ q(func: allof(name, rune, "ron")) {
					name
				}}`, `
				{ "q": [
					{ "name": "Aaron" }
				]}`,
			},
			{`
				{ q(func: allof(name, rune, "ron")) {
					name
				}}`, `
				{ "q": [
					{ "name": "Aaron" }
				]}`,
			},
			{`
				{ q(func: allof(name, rune, "no")) {
					name
				}}`, `
				{ "q": [
					{ "name": "Ronald" },
					{ "name": "Aaron" }
				]}`,
			},
			{`
				{ q(func: anyof(name, rune, "mr")) {
					name
				}}`, `
				{ "q": [
					{ "name": "Adam" },
					{ "name": "Aaron" },
					{ "name": "Amy" }
				]}`,
			},
			{`
				{ q(func: anyof(name, rune, "da")) {
					name
				}}`, `
				{ "q": [
					{ "name": "Adam" },
					{ "name": "Aaron" },
					{ "name": "Ronald" }
				]}`,
			},
		},
	},
	{
		"num: int @index(factor) .",
		`[
			{ "num": 2 },
			{ "num": 3 },
			{ "num": 4 },
			{ "num": 5 },
			{ "num": 6 },
			{ "num": 7 },
			{ "num": 8 },
			{ "num": 9 },
			{ "num": 10 },
			{ "num": 11 },
			{ "num": 12 },
			{ "num": 13 },
			{ "num": 14 },
			{ "num": 15 },
			{ "num": 16 },
			{ "num": 17 },
			{ "num": 18 },
			{ "num": 19 },
			{ "num": 20 },
			{ "num": 21 },
			{ "num": 22 },
			{ "num": 23 },
			{ "num": 24 },
			{ "num": 25 },
			{ "num": 26 },
			{ "num": 27 },
			{ "num": 28 },
			{ "num": 29 },
			{ "num": 30 }
		]`,
		[]testCase{
			{`
				{
					q(func: allof(num, factor, 5)) {
						num
					}
				}`, `
				{ "q": [
					{ "num": 15 },
					{ "num": 20 },
					{ "num": 5 },
					{ "num": 10 },
					{ "num": 30 },
					{ "num": 25 }
				]}`,
			},
			{`
				{
					q(func: allof(num, factor, 15)) {
						num
					}
				}`, `
				{ "q": [
					{ "num": 15 },
					{ "num": 30 }
				]}`,
			},
			{`
				{
					q(func: anyof(num, factor, 15)) {
						num
					}
				}`, `
				{ "q": [
					{ "num": 3 },
					{ "num": 5 },
					{ "num": 6 },
					{ "num": 9 },
					{ "num": 10 },
					{ "num": 12 },
					{ "num": 15 },
					{ "num": 18 },
					{ "num": 20 },
					{ "num": 21 },
					{ "num": 24 },
					{ "num": 25 },
					{ "num": 27 },
					{ "num": 30 }
				]}`,
			},
		},
	},
}

func (psuite *PluginTestSuite) TestPlugins() {
	t := psuite.T()

	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	for i := 0; i < len(testInp); i++ {
		psuite.dataSetNdx = i
		psuite.Run(fmt.Sprintf("test case %d", i+1), psuite.pluginFn)
	}
}

func (psuite *PluginTestSuite) pluginFn() {
	t := psuite.T()
	gcli, cleanup, err := psuite.dc.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(),
		dgraphtest.DefaultUser, dgraphtest.DefaultPassword, x.GalaxyNamespace))
	require.NoError(t, gcli.DropAll())
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	fmt.Printf("\ndataSetNdx = %d\n", psuite.dataSetNdx)
	fmt.Printf("\ntestInp[psuite.dataSetNdx].initialSchema = [%s]\n", testInp[psuite.dataSetNdx].initialSchema)
	require.NoError(t, gcli.Alter(ctx, &api.Operation{
		Schema: testInp[psuite.dataSetNdx].initialSchema,
	}))

	txn := gcli.NewTxn()
	_, err = txn.Mutate(ctx, &api.Mutation{SetJson: []byte(testInp[psuite.dataSetNdx].setJSON)})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	for _, test := range testInp[psuite.dataSetNdx].cases {
		txn := gcli.NewTxn()
		reply, err := txn.Query(ctx, test.query)
		require.NoError(t, err)
		dgraphtest.CompareJSON(test.wantResult, string(reply.GetJson()))
	}
}
