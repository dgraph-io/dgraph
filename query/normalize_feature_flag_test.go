//go:build integration2

/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors *
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

package query

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v240/protos/api"
	"github.com/dgraph-io/dgraph/v24/dgraphtest"
)

func TestNormalizeDirectiveWithNoListResponse(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).
		WithReplicas(1).WithNormalizeCompatibilityMode("v20")
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, c.AssignUids(gc.Dgraph, 100))

	const dataSchema = `
        friend : [uid] @reverse @count .
        name   : string @index(term, exact, trigram) @count @lang .
        dob    : dateTime @index(year) .`
	require.NoError(t, gc.SetupSchema(dataSchema))

	triples := []byte(`
        <1> <friend> <23> .
        <1> <friend> <24> .
        <1> <friend> <25> .
        <1> <friend> <31> .
        <1> <friend> <101>.
        <23> <friend> <1> .
        <31> <friend> <1> .
        <31> <friend> <25> .
        <1> <dob> "1910-01-01" .
		<23> <dob> "1910-01-02" .
		<24> <dob> "1909-05-05" .
		<25> <dob> "1909-01-10" .
		<31> <dob> "1901-01-15" .
        <1> <name> "Michonne" .
        <23> <name> "Rick Grimes" .
        <24> <name> "Glenn Rhee" .`)
	_, err = gc.Mutate(&api.Mutation{SetNquads: triples, CommitNow: true})
	require.NoError(t, err)

	query := `
		{
			me(func: uid(0x01)) @recurse @normalize {
				n: name
				d: dob
				friend
			}
		}`
	js, err := gc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, `
        {
            "me": [
                {
                    "n": "Michonne",
                    "d": "1910-01-01T00:00:00Z",
                    "n": "Rick Grimes",
                    "d": "1910-01-02T00:00:00Z",
                    "n": "Michonne",
                    "d": "1910-01-01T00:00:00Z"
                },
                {
                    "n": "Michonne",
                    "d": "1910-01-01T00:00:00Z",
                    "n": "Glenn Rhee",
                    "d": "1909-05-05T00:00:00Z"
                },
                {
                    "n": "Michonne",
                    "d": [
                        "1910-01-01T00:00:00Z",
                        "1909-01-10T00:00:00Z"
                    ]
                },
                {
                    "n": "Michonne",
                    "d": [
                        "1910-01-01T00:00:00Z",
                        "1901-01-15T00:00:00Z"
                    ],
                    "n": "Michonne",
                    "d": "1910-01-01T00:00:00Z"
                },
                {
                    "n": "Michonne",
                    "d": [
                        "1910-01-01T00:00:00Z",
                        "1901-01-15T00:00:00Z",
                        "1909-01-10T00:00:00Z"
                    ]
                }
            ]
        }`, string(js.Json))
}
