//go:build integration || upgrade

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"fmt"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/hypermodeinc/dgraph/v25/dgraphapi"
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
	for i := 0; i < len(testInp); i++ {
		psuite.Run(fmt.Sprintf("test case %d", i+1), func() {
			t := psuite.T()
			gcli, cleanup, err := psuite.dc.Client()
			require.NoError(t, err)
			defer cleanup()
			require.NoError(t, gcli.DropAll())
			require.NoError(t, gcli.SetupSchema(testInp[i].initialSchema))
			_, err = gcli.Mutate(&api.Mutation{
				SetJson:   []byte(testInp[i].setJSON),
				CommitNow: true,
			})
			require.NoError(t, err)

			// Upgrade
			psuite.Upgrade()

			gcli, cleanup, err = psuite.dc.Client()
			require.NoError(t, err)
			defer cleanup()

			for _, test := range testInp[i].cases {
				reply, err := gcli.Query(test.query)
				require.NoError(t, err)
				dgraphapi.CompareJSON(test.wantResult, string(reply.GetJson()))
			}
		})
	}
}
