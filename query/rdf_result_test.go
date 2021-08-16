/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package query

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRDFResult(t *testing.T) {
	query := `{
		friends_15_and_19(func: uid(1)) {
		  name
		  friend @filter(ge(age, 15) AND lt(age, 19)) {
			  name
			  age
		  }
		}
	  }`

	output, err := processQueryRDF(context.Background(), t, query)
	require.NoError(t, err)
	rdfs := []string{`<0x1> <name> "Michonne" .`,
		`<0x1> <friend> <0x17> .`,
		`<0x1> <friend> <0x18> .`,
		`<0x1> <friend> <0x19> .`,
		`<0x17> <name> "Rick Grimes" .`,
		`<0x18> <name> "Glenn Rhee" .`,
		`<0x19> <name> "Daryl Dixon" .`,
		`<0x17> <age> "15" .`,
		`<0x18> <age> "15" .`,
		`<0x19> <age> "17" .`,
	}
	// TODO: We should do both size equality check.
	for _, rdf := range rdfs {
		require.Contains(t, output, rdf)
	}
}

func TestRDFNormalize(t *testing.T) {
	query := `
	{
		me(func: uid(0x01)) @normalize {
			mn: name
			gender
			friend {
				n: name
				d: dob
				friend {
					fn : name
				}
			}
			son {
				sn: name
			}
		}
	}`
	_, err := processQueryRDF(context.Background(), t, query)
	require.Error(t, err, "normalize directive is not supported in the rdf output format")
}

func TestRDFGroupBy(t *testing.T) {
	query := `
	{
		me(func: uid(1, 23, 24, 25, 31)) @groupby(age) {
				count(uid)
		}
	}`
	_, err := processQueryRDF(context.Background(), t, query)
	require.Contains(t, err.Error(), "groupby is not supported in rdf output format")
}

func TestRDFUidCount(t *testing.T) {
	query := `
	{
		me(func: gt(count(friend), 0)) {
			count(uid)
		}
	}`
	_, err := processQueryRDF(context.Background(), t, query)
	require.Contains(t, err.Error(), "uid count is not supported in the rdf output format")
}

func TestRDFIngoreReflex(t *testing.T) {
	query := `
	{
		me(func:anyofterms(name, "Michonne Rick Daryl")) @ignoreReflex {
			name
			friend {
				name
				friend {
					name
				}
			}
		}
	}`
	_, err := processQueryRDF(context.Background(), t, query)
	require.Contains(t, err.Error(),
		"ignorereflex directive is not supported in the rdf output format")
}

func TestRDFRecurse(t *testing.T) {
	query := `
	{
		me(func: anyofterms(name, "Michonne Rick Daryl")) @recurse(depth: 1, loop: true) {
			name
			friend
		}
	}`
	rdf, err := processQueryRDF(context.Background(), t, query)
	require.NoError(t, err)
	require.Equal(t, rdf, `<0x1> <name> "Michonne" .
<0x17> <name> "Rick Grimes" .
<0x19> <name> "Daryl Dixon" .
`)
}

func TestRDFIgnoreUid(t *testing.T) {
	query := `
	{
		me(func: anyofterms(name, "Michonne Rick Daryl")) {
			uid
			name
		}
	}`
	rdf, err := processQueryRDF(context.Background(), t, query)
	require.NoError(t, err)
	require.Equal(t, rdf, `<0x1> <name> "Michonne" .
<0x17> <name> "Rick Grimes" .
<0x19> <name> "Daryl Dixon" .
`)
}

func TestRDFCheckPwd(t *testing.T) {
	query := `
    {
        me(func: uid(0x01)) {
			expand(_all_)
			checkpwd(password, "12345")
		}
    }
	`
	_, err := processQueryRDF(context.Background(), t, query)
	require.Contains(t, err.Error(),
		"chkpwd function is not supported in the rdf output format")
}

func TestRDFPredicateCount(t *testing.T) {
	query := `
    {
		me(func:anyofterms(name, "Michonne Rick Daryl")) {
			name
			count(friend)
			friend {
				name
			}
		}
    }
	`

	output, err := processQueryRDF(context.Background(), t, query)
	require.NoError(t, err)
	rdfs := []string{
		`<0x1> <name> "Michonne" .`,
		`<0x17> <name> "Rick Grimes" .`,
		`<0x19> <name> "Daryl Dixon" .`,
		`<0x1> <count(friend)> "5" .`,
		`<0x17> <count(friend)> "1" .`,
		`<0x19> <count(friend)> "0" .`,
		`<0x1> <friend> <0x17> .`,
		`<0x1> <friend> <0x18> .`,
		`<0x1> <friend> <0x19> .`,
		`<0x1> <friend> <0x1f> .`,
		`<0x1> <friend> <0x65> .`,
		`<0x17> <friend> <0x1> .`,
		`<0x1> <name> "Michonne" .`,
		`<0x17> <name> "Rick Grimes" .`,
		`<0x18> <name> "Glenn Rhee" .`,
		`<0x19> <name> "Daryl Dixon" .`,
		`<0x1f> <name> "Andrea" .`,
	}
	for _, rdf := range rdfs {
		require.Contains(t, output, rdf)
	}
}

func TestRDFFacets(t *testing.T) {
	query := `
		{
			shortest(from: 1, to:1001, numpaths: 4) {
				path @facets(weight)
			}
		}`
	_, err := processQueryRDF(context.Background(), t, query)
	require.Contains(t, err.Error(),
		"facets are not supported in the rdf output format")
}

func TestDateRDF(t *testing.T) {
	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend(orderdesc: film.film.initial_release_date) {
					name
					film.film.initial_release_date
				}
			}
		}
	`
	output, err := processQueryRDF(context.Background(), t, query)
	require.NoError(t, err)
	rdfs := []string{
		`<0x1> <name> "Michonne" .`,
		`<0x1> <gender> "female" .`,
		`<0x1> <friend> <0x19> .`,
		`<0x1> <friend> <0x18> .`,
		`<0x1> <friend> <0x17> .`,
		`<0x1> <friend> <0x1f> .`,
		`<0x1> <friend> <0x65> .`,
		`<0x17> <name> "Rick Grimes" .`,
		`<0x18> <name> "Glenn Rhee" .`,
		`<0x19> <name> "Daryl Dixon" .`,
		`<0x1f> <name> "Andrea" .`,
		`<0x17> <film.film.initial_release_date> "1900-01-02T00:00:00Z" .`,
		`<0x18> <film.film.initial_release_date> "1909-05-05T00:00:00Z" .`,
		`<0x19> <film.film.initial_release_date> "1929-01-10T00:00:00Z" .`,
		`<0x1f> <film.film.initial_release_date> "1801-01-15T00:00:00Z" .`,
	}
	for _, rdf := range rdfs {
		require.Contains(t, output, rdf)
	}
}
