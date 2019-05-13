/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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
	"time"

	"github.com/stretchr/testify/require"
)

func TestToFastJSONFilterUID(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(anyofterms(name, "Andrea")) {
					uid
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne","gender":"female","friend":[{"uid":"0x1f"}]}]}}`,
		js)
}

func TestToFastJSONFilterOrUID(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(anyofterms(name, "Andrea") or anyofterms(name, "Andrea Rhee")) {
					uid
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne","gender":"female","friend":[{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x1f","name":"Andrea"}]}]}}`,
		js)
}

func TestToFastJSONFilterOrCount(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				count(friend @filter(anyofterms(name, "Andrea") or anyofterms(name, "Andrea Rhee")))
				friend @filter(anyofterms(name, "Andrea")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"count(friend)":2,"friend": [{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONFilterOrFirst(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend(first:2) @filter(anyofterms(name, "Andrea") or anyofterms(name, "Glenn SomethingElse") or anyofterms(name, "Daryl")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Glenn Rhee"},{"name":"Daryl Dixon"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONFilterOrOffset(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend(offset:1) @filter(anyofterms(name, "Andrea") or anyofterms(name, "Glenn Rhee") or anyofterms(name, "Daryl Dixon")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Daryl Dixon"},{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONFiltergeName(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				friend @filter(ge(name, "Rick")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Rick Grimes"}]}]}}`,
		js)
}

func TestToFastJSONFilterLtAlias(t *testing.T) {

	// We shouldn't get Zambo Alice.
	query := `
		{
			me(func: uid(0x01)) {
				friend(orderasc: alias) @filter(lt(alias, "Pat")) {
					alias
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"alias":"Allan Matt"},{"alias":"Bob Joe"},{"alias":"John Alice"},{"alias":"John Oliver"}]}]}}`,
		js)
}

func TestToFastJSONFilterge1(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(ge(dob, "1909-05-05")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONFilterge2(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(ge(dob_day, "1909-05-05")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONFilterGt(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(gt(dob, "1909-05-05")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Rick Grimes"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONFilterle(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(le(dob, "1909-01-10")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Daryl Dixon"},{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONFilterLt(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(lt(dob, "1909-01-10")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONFilterEqualNoHit(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(eq(dob, "1909-03-20")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"gender":"female","name":"Michonne"}]}}`,
		js)
}
func TestToFastJSONFilterEqualName(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(eq(name, "Daryl Dixon")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Daryl Dixon"}], "gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONFilterEqualNameNoHit(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(eq(name, "Daryl")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONFilterEqual(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(eq(dob, "1909-01-10")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Daryl Dixon"}], "gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONOrderName(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				friend(orderasc: alias) {
					alias
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"alias":"Allan Matt"},{"alias":"Bob Joe"},{"alias":"John Alice"},{"alias":"John Oliver"},{"alias":"Zambo Alice"}],"name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONOrderNameDesc(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				friend(orderdesc: alias) {
					alias
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"alias":"Zambo Alice"},{"alias":"John Oliver"},{"alias":"John Alice"},{"alias":"Bob Joe"},{"alias":"Allan Matt"}],"name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONOrderName1(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				friend(orderasc: name ) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Andrea"},{"name":"Daryl Dixon"},{"name":"Glenn Rhee"},{"name":"Rick Grimes"}],"name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONOrderNameError(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				friend(orderasc: nonexistent) {
					name
				}
			}
		}
	`
	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
}

func TestToFastJSONFilterleOrder(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend(orderasc: dob) @filter(le(dob, "1909-03-20")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Andrea"},{"name":"Daryl Dixon"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONFiltergeNoResult(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(ge(dob, "1999-03-20")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"gender":"female","name":"Michonne"}]}}`, js)
}

func TestToFastJSONFirstOffsetOutOfBound(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend(offset:100, first:1) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"gender":"female","name":"Michonne"}]}}`,
		js)
}

// No filter. Just to test first and offset.
func TestToFastJSONFirstOffset(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend(offset:1, first:1) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Glenn Rhee"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONFilterOrFirstOffset(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend(offset:1, first:1) @filter(anyofterms(name, "Andrea") or anyofterms(name, "SomethingElse Rhee") or anyofterms(name, "Daryl Dixon")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Daryl Dixon"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONFilterleFirstOffset(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend(offset:1, first:1) @filter(le(dob, "1909-03-20")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONFilterOrFirstOffsetCount(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				count(friend(offset:1, first:1) @filter(anyofterms(name, "Andrea") or anyofterms(name, "SomethingElse Rhee") or anyofterms(name, "Daryl Dixon")))
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"count(friend)":1,"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONFilterOrFirstNegative(t *testing.T) {

	// When negative first/count is specified, we ignore offset and returns the last
	// few number of items.
	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend(first:-1, offset:0) @filter(anyofterms(name, "Andrea") or anyofterms(name, "Glenn Rhee") or anyofterms(name, "Daryl Dixon")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONFilterNot1(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(not anyofterms(name, "Andrea rick")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"gender":"female","name":"Michonne","friend":[{"name":"Glenn Rhee"},{"name":"Daryl Dixon"}]}]}}`, js)
}

func TestToFastJSONFilterNot2(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(not anyofterms(name, "Andrea") and anyofterms(name, "Glenn Andrea")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"gender":"female","name":"Michonne","friend":[{"name":"Glenn Rhee"}]}]}}`, js)
}

func TestToFastJSONFilterNot3(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(not (anyofterms(name, "Andrea") or anyofterms(name, "Glenn Rick Andrea"))) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"gender":"female","name":"Michonne","friend":[{"name":"Daryl Dixon"}]}]}}`, js)
}

func TestToFastJSONFilterNot4(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend (first:2) @filter(not anyofterms(name, "Andrea")
				and not anyofterms(name, "glenn")
				and not anyofterms(name, "rick")
			) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"gender":"female","name":"Michonne","friend":[{"name":"Daryl Dixon"}]}]}}`, js)
}

// TestToFastJSONFilterNot4 was unstable (fails observed locally and on travis).
// Following method repeats the query to make sure that it never fails.
// It's commented out, because it's too slow for everyday testing.
/*
func TestToFastJSONFilterNot4x1000000(t *testing.T) {

	for i := 0; i < 1000000; i++ {
		query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend (first:2) @filter(not anyofterms(name, "Andrea")
				and not anyofterms(name, "glenn")
				and not anyofterms(name, "rick")
			) {
				name
			}
		}
	}
	`

		js := processQueryNoErr(t, query)
		require.JSONEq(t,
			`{"data": {"me":[{"gender":"female","name":"Michonne","friend":[{"name":"Daryl Dixon"}]}]}}`, js,
			"tzdybal: %d", i)
	}
}
*/

func TestToFastJSONFilterAnd(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(anyofterms(name, "Andrea") and anyofterms(name, "SomethingElse Rhee")) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne","gender":"female"}]}}`, js)
}

func TestCountReverseFunc(t *testing.T) {

	query := `
		{
			me(func: ge(count(~friend), 2)) {
				name
				count(~friend)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Glenn Rhee","count(~friend)":2}]}}`,
		js)
}

func TestCountReverseFilter(t *testing.T) {

	query := `
		{
			me(func: anyofterms(name, "Glenn Michonne Rick")) @filter(ge(count(~friend), 2)) {
				name
				count(~friend)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Glenn Rhee","count(~friend)":2}]}}`,
		js)
}

func TestCountReverse(t *testing.T) {

	query := `
		{
			me(func: uid(0x18)) {
				name
				count(~friend)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Glenn Rhee","count(~friend)":2}]}}`,
		js)
}

func TestToFastJSONReverse(t *testing.T) {

	query := `
		{
			me(func: uid(0x18)) {
				name
				~friend {
					name
					gender
					alive
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Glenn Rhee","~friend":[{"alive":true,"gender":"female","name":"Michonne"},{"alive": false, "name":"Andrea"}]}]}}`,
		js)
}

func TestToFastJSONReverseFilter(t *testing.T) {

	query := `
		{
			me(func: uid(0x18)) {
				name
				~friend @filter(allofterms(name, "Andrea")) {
					name
					gender
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Glenn Rhee","~friend":[{"name":"Andrea"}]}]}}`,
		js)
}

// Test sorting / ordering by dob.
func TestToFastJSONOrder(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend(orderasc: dob) {
					name
					dob
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne","gender":"female","friend":[{"name":"Andrea","dob":"1901-01-15T00:00:00Z"},{"name":"Daryl Dixon","dob":"1909-01-10T00:00:00Z"},{"name":"Glenn Rhee","dob":"1909-05-05T00:00:00Z"},{"name":"Rick Grimes","dob":"1910-01-02T00:00:00Z"}]}]}}`,
		js)
}

// Test sorting / ordering by dob.
func TestToFastJSONOrderDesc1(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend(orderdesc: dob) {
					name
					dob
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"dob":"1910-01-02T00:00:00Z","name":"Rick Grimes"},{"dob":"1909-05-05T00:00:00Z","name":"Glenn Rhee"},{"dob":"1909-01-10T00:00:00Z","name":"Daryl Dixon"},{"dob":"1901-01-15T00:00:00Z","name":"Andrea"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestToFastJSONOrderDesc2(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend(orderdesc: dob_day) {
					name
					dob
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"dob":"1910-01-02T00:00:00Z","name":"Rick Grimes"},{"dob":"1909-05-05T00:00:00Z","name":"Glenn Rhee"},{"dob":"1909-01-10T00:00:00Z","name":"Daryl Dixon"},{"dob":"1901-01-15T00:00:00Z","name":"Andrea"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

// Test sorting / ordering by dob.
func TestToFastJSONOrderDesc_pawan(t *testing.T) {

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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"film.film.initial_release_date":"1929-01-10T00:00:00Z","name":"Daryl Dixon"},{"film.film.initial_release_date":"1909-05-05T00:00:00Z","name":"Glenn Rhee"},{"film.film.initial_release_date":"1900-01-02T00:00:00Z","name":"Rick Grimes"},{"film.film.initial_release_date":"1801-01-15T00:00:00Z","name":"Andrea"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

// Test sorting / ordering by dob.
func TestToFastJSONOrderDedup(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				friend(orderasc: name) {
					dob
					name
				}
				gender
				name
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"dob":"1901-01-15T00:00:00Z","name":"Andrea"},{"dob":"1909-01-10T00:00:00Z","name":"Daryl Dixon"},{"dob":"1909-05-05T00:00:00Z","name":"Glenn Rhee"},{"dob":"1910-01-02T00:00:00Z","name":"Rick Grimes"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

// Test sorting / ordering by dob and count.
func TestToFastJSONOrderDescCount(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				count(friend @filter(anyofterms(name, "Rick")) (orderasc: dob))
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"count(friend)":1,"gender":"female","name":"Michonne"}]}}`,
		js)
}

// Test sorting / ordering by dob.
func TestToFastJSONOrderOffset(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend(orderasc: dob, offset: 2) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Glenn Rhee"},{"name":"Rick Grimes"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

// Test sorting / ordering by dob.
func TestToFastJSONOrderOffsetCount(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend(orderasc: dob, offset: 2, first: 1) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Glenn Rhee"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestSchema1(t *testing.T) {

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			person(func: uid(0x01)) {
				name
				age
				address
				alive
				survival_rate
				friend {
					name
					address
					age
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"person":[{"address":"31, 32 street, Jupiter","age":38,"alive":true,"friend":[{"address":"21, mark street, Mars","age":15,"name":"Rick Grimes"},{"name":"Glenn Rhee","age":15},{"age":17,"name":"Daryl Dixon"},{"age":19,"name":"Andrea"}],"name":"Michonne","survival_rate":98.990000}]}}`, js)
}

func TestMultiQuery(t *testing.T) {

	query := `
		{
			me(func:anyofterms(name, "Michonne")) {
				name
				gender
			}

			you(func:anyofterms(name, "Andrea")) {
				name
			}
		}
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"gender":"female","name":"Michonne"}],"you":[{"name":"Andrea"},{"name":"Andrea With no friends"}]}}`, js)
}

func TestMultiQueryError1(t *testing.T) {

	query := `
    {
			me(func:anyofterms(name, "Michonne")) {
        name
        gender

			you(func:anyofterms(name, "Andrea")) {
        name
      }
    }
  `
	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
}

func TestMultiQueryError2(t *testing.T) {

	query := `
    {
      me(anyofterms(name, "Michonne")) {
        name
        gender
			}
		}

      you(anyofterms(name, "Andrea")) {
        name
      }
    }
  `
	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
}

func TestGenerator(t *testing.T) {

	query := `
    {
			me(func:anyofterms(name, "Michonne")) {
        name
        gender
      }
    }
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"gender":"female","name":"Michonne"}]}}`, js)
}

func TestGeneratorMultiRootMultiQueryRootval(t *testing.T) {

	query := `
    {
			friend as var(func:anyofterms(name, "Michonne Rick Glenn")) {
      	name
			}

			you(func: uid(friend)) {
				name
			}
    }
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"you":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"}]}}`, js)
}

func TestGeneratorMultiRootMultiQueryVarFilter(t *testing.T) {

	query := `
    {
			f as var(func:anyofterms(name, "Michonne Rick Glenn")) {
      			name
			}

			you(func:anyofterms(name, "Michonne")) {
				friend @filter(uid(f)) {
					name
				}
			}
    }
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"you":[{"friend":[{"name":"Rick Grimes"}, {"name":"Glenn Rhee"}]}]}}`, js)
}

func TestGeneratorMultiRootMultiQueryRootVarFilter(t *testing.T) {

	query := `
    {
			friend as var(func:anyofterms(name, "Michonne Rick Glenn")) {
			}

			you(func:anyofterms(name, "Michonne Andrea Glenn")) @filter(uid(friend)) {
				name
			}
    }
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"you":[{"name":"Michonne"}, {"name":"Glenn Rhee"}]}}`, js)
}

func TestGeneratorMultiRootMultiQuery(t *testing.T) {

	query := `
    {
			me(func:anyofterms(name, "Michonne Rick Glenn")) {
        name
      }

			you(func: uid(1, 23, 24)) {
				name
			}
    }
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"}], "you":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"}]}}`, js)
}

func TestGeneratorMultiRootVarOrderOffset(t *testing.T) {

	query := `
    {
			L as var(func:anyofterms(name, "Michonne Rick Glenn"), orderasc: dob, offset:2) {
        name
      }

			me(func: uid(L)) {
			 name
			}
    }
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Rick Grimes"}]}}`, js)
}

func TestGeneratorMultiRootVarOrderOffset1(t *testing.T) {

	query := `
    {
			me(func:anyofterms(name, "Michonne Rick Glenn"), orderasc: dob, offset:2) {
        name
      }
    }
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Rick Grimes"}]}}`, js)
}

func TestGeneratorMultiRootOrderOffset(t *testing.T) {

	query := `
    {
			L as var(func:anyofterms(name, "Michonne Rick Glenn")) {
        name
      }
			me(func: uid(L), orderasc: dob, offset:2) {
        name
      }
    }
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Rick Grimes"}]}}`, js)
}

func TestGeneratorMultiRootOrderdesc(t *testing.T) {

	query := `
    {
			me(func:anyofterms(name, "Michonne Rick Glenn"), orderdesc: dob) {
        name
      }
    }
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Rick Grimes"},{"name":"Michonne"},{"name":"Glenn Rhee"}]}}`, js)
}

func TestGeneratorMultiRootOrder(t *testing.T) {

	query := `
    {
			me(func:anyofterms(name, "Michonne Rick Glenn"), orderasc: dob) {
        name
      }
    }
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Glenn Rhee"},{"name":"Michonne"},{"name":"Rick Grimes"}]}}`, js)
}

func TestGeneratorMultiRootOffset(t *testing.T) {

	query := `
    {
			me(func:anyofterms(name, "Michonne Rick Glenn"), offset: 1) {
        name
      }
    }
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"}]}}`, js)
}

func TestGeneratorMultiRoot(t *testing.T) {

	query := `
    {
			me(func:anyofterms(name, "Michonne Rick Glenn")) {
        name
      }
    }
  `

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"},{"name":"Rick Grimes"},
		{"name":"Glenn Rhee"}]}}`, js)
}

func TestRootList(t *testing.T) {
	query := `{
	me(func: uid(1, 23, 24)) {
		name
	}
}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"}]}}`, js)
}

func TestRootList1(t *testing.T) {

	query := `{
	me(func: uid(0x01, 23, 24, 110)) {
		name
	}
}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne"},{"name":"Rick Grimes"}`+
			`,{"name":"Glenn Rhee"},{"name":"Alice"}]}}`, js)
}

func TestRootList2(t *testing.T) {

	query := `{
	me(func: uid(0x01, 23, 110, 24)) {
		name
	}
}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"},{"name":"Rick Grimes"}`+
		`,{"name":"Glenn Rhee"},{"name":"Alice"}]}}`, js)
}

func TestGeneratorMultiRootFilter1(t *testing.T) {

	query := `
    {
			me(func:anyofterms(name, "Daryl Rick Glenn")) @filter(le(dob, "1909-01-10")) {
        name
      }
    }
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Daryl Dixon"}]}}`, js)
}

func TestGeneratorMultiRootFilter2(t *testing.T) {

	query := `
    {
			me(func:anyofterms(name, "Michonne Rick Glenn")) @filter(ge(dob, "1909-01-10")) {
        name
      }
    }
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"},{"name":"Rick Grimes"}`+
		`,{"name":"Glenn Rhee"}]}}`, js)
}

func TestGeneratorMultiRootFilter3(t *testing.T) {

	query := `
    {
		me(func:anyofterms(name, "Michonne Rick Glenn")) @filter(anyofterms(name, "Glenn") and ge(dob, "1909-01-10")) {
        name
      }
    }
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Glenn Rhee"}]}}`, js)
}

func TestGeneratorRootFilterOnCountGt(t *testing.T) {

	query := `
                {
                        me(func:anyofterms(name, "Michonne Rick")) @filter(gt(count(friend), 2)) {
                                name
                        }
                }
        `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"}]}}`, js)
}

func TestGeneratorRootFilterOnCountle(t *testing.T) {

	query := `
                {
                        me(func:anyofterms(name, "Michonne Rick")) @filter(le(count(friend), 2)) {
                                name
                        }
                }
        `

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Rick Grimes"}]}}`, js)
}

func TestGeneratorRootFilterOnCountChildLevel(t *testing.T) {

	query := `
                {
                        me(func: uid(23)) {
                                name
                                friend @filter(gt(count(friend), 2)) {
                                        name
                                }
                        }
                }
        `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"friend":[{"name":"Michonne"}],"name":"Rick Grimes"}]}}`, js)
}

func TestGeneratorRootFilterOnCountWithAnd(t *testing.T) {

	query := `
                {
                        me(func: uid(23)) {
                                name
                                friend @filter(gt(count(friend), 4) and lt(count(friend), 100)) {
                                        name
                                }
                        }
                }
        `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"friend":[{"name":"Michonne"}],"name":"Rick Grimes"}]}}`, js)
}

func TestGeneratorRootFilterOnCountError1(t *testing.T) {

	// only cmp(count(attr), int) is valid, 'max'/'min'/'sum' not supported
	query := `
                {
                        me(func:anyofterms(name, "Michonne Rick")) @filter(gt(count(friend), "invalid")) {
                                name
                        }
                }
        `

	_, err := processQuery(context.Background(), t, query)
	require.NotNil(t, err)
}

func TestGeneratorRootFilterOnCountError2(t *testing.T) {

	// missing digits
	query := `
                {
                        me(func:anyofterms(name, "Michonne Rick")) @filter(gt(count(friend))) {
                                name
                        }
                }
        `

	_, err := processQuery(context.Background(), t, query)
	require.NotNil(t, err)
}

func TestGeneratorRootFilterOnCountError3(t *testing.T) {

	// to much args
	query := `
                {
                        me(func:anyofterms(name, "Michonne Rick")) @filter(gt(count(friend), 2, 4)) {
                                name
                        }
                }
        `

	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
}

func TestNearGenerator(t *testing.T) {

	time.Sleep(10 * time.Millisecond)
	query := `{
		me(func:near(loc, [1.1,2.0], 5.001)) @filter(not uid(25)) {
			name
			gender
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne","gender":"female"},{"name":"Rick Grimes","gender": "male"},{"name":"Glenn Rhee"}]}}`, js)
}

func TestNearGeneratorFilter(t *testing.T) {

	query := `{
		me(func:near(loc, [1.1,2.0], 5.001)) @filter(allofterms(name, "Michonne")) {
			name
			gender
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"gender":"female","name":"Michonne"}]}}`, js)
}

func TestNearGeneratorError(t *testing.T) {

	query := `{
		me(func:near(loc, [1.1,2.0], -5.0)) {
			name
			gender
		}
	}`

	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
}

func TestNearGeneratorErrorMissDist(t *testing.T) {

	query := `{
		me(func:near(loc, [1.1,2.0])) {
			name
			gender
		}
	}`

	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
}

func TestWithinGeneratorError(t *testing.T) {

	query := `{
		me(func:within(loc, [[[0.0,0.0], [2.0,0.0], [1.5, 3.0], [0.0, 2.0], [0.0, 0.0]]], 12.2)) {
			name
			gender
		}
	}`

	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
}

func TestWithinGenerator(t *testing.T) {

	query := `{
		me(func:within(loc,  [[[0.0,0.0], [2.0,0.0], [1.5, 3.0], [0.0, 2.0], [0.0, 0.0]]])) @filter(not uid(25)) {
			name
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"},{"name":"Glenn Rhee"}]}}`, js)
}

func TestContainsGenerator(t *testing.T) {

	query := `{
		me(func:contains(loc, [2.0,0.0])) {
			name
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Rick Grimes"}]}}`, js)
}

func TestContainsGenerator2(t *testing.T) {

	query := `{
		me(func:contains(loc,  [[[1.0,1.0], [1.9,1.0], [1.9, 1.9], [1.0, 1.9], [1.0, 1.0]]])) {
			name
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Rick Grimes"}]}}`, js)
}

func TestIntersectsGeneratorError(t *testing.T) {

	query := `{
		me(func:intersects(loc, [0.0,0.0])) {
			name
		}
	}`

	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
}

func TestIntersectsGenerator(t *testing.T) {

	query := `{
		me(func:intersects(loc, [[[0.0,0.0], [2.0,0.0], [1.5, 3.0], [0.0, 2.0], [0.0, 0.0]]])) @filter(not uid(25)) {
			name
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"}, {"name":"Rick Grimes"}, {"name":"Glenn Rhee"}]}}`, js)
}

// this test is failing when executed alone, but pass when executed after other tests
// TODO: find and remove the dependency
func TestNormalizeDirective(t *testing.T) {
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
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"d":"1910-01-02T00:00:00Z","fn":"Michonne","mn":"Michonne","n":"Rick Grimes","sn":"Andre"},{"d":"1910-01-02T00:00:00Z","fn":"Michonne","mn":"Michonne","n":"Rick Grimes","sn":"Helmut"},{"d":"1909-05-05T00:00:00Z","mn":"Michonne","n":"Glenn Rhee","sn":"Andre"},{"d":"1909-05-05T00:00:00Z","mn":"Michonne","n":"Glenn Rhee","sn":"Helmut"},{"d":"1909-01-10T00:00:00Z","mn":"Michonne","n":"Daryl Dixon","sn":"Andre"},{"d":"1909-01-10T00:00:00Z","mn":"Michonne","n":"Daryl Dixon","sn":"Helmut"},{"d":"1901-01-15T00:00:00Z","fn":"Glenn Rhee","mn":"Michonne","n":"Andrea","sn":"Andre"},{"d":"1901-01-15T00:00:00Z","fn":"Glenn Rhee","mn":"Michonne","n":"Andrea","sn":"Helmut"}]}}`,
		js)
}

func TestNearPoint(t *testing.T) {

	query := `{
		me(func: near(geometry, [-122.082506, 37.4249518], 1)) {
			name
		}
	}`

	js := processQueryNoErr(t, query)
	expected := `{"data": {"me":[{"name":"Googleplex"},{"name":"SF Bay area"},{"name":"Mountain View"}]}}`
	require.JSONEq(t, expected, js)
}

func TestWithinPolygon(t *testing.T) {

	query := `{
		me(func: within(geometry, [[[-122.06, 37.37], [-122.1, 37.36], [-122.12, 37.4], [-122.11, 37.43], [-122.04, 37.43], [-122.06, 37.37]]])) {
			name
		}
	}`
	js := processQueryNoErr(t, query)
	expected := `{"data": {"me":[{"name":"Googleplex"},{"name":"Shoreline Amphitheater"}]}}`
	require.JSONEq(t, expected, js)
}

func TestContainsPoint(t *testing.T) {

	query := `{
		me(func: contains(geometry, [-122.082506, 37.4249518])) {
			name
		}
	}`

	js := processQueryNoErr(t, query)
	expected := `{"data": {"me":[{"name":"SF Bay area"},{"name":"Mountain View"}]}}`
	require.JSONEq(t, expected, js)
}

func TestNearPoint2(t *testing.T) {

	query := `{
		me(func: near(geometry, [-122.082506, 37.4249518], 1000)) {
			name
		}
	}`

	js := processQueryNoErr(t, query)
	expected := `{"data": {"me":[{"name":"Googleplex"},{"name":"Shoreline Amphitheater"}, {"name": "SF Bay area"}, {"name": "Mountain View"}]}}`
	require.JSONEq(t, expected, js)
}

func TestIntersectsPolygon1(t *testing.T) {

	query := `{
		me(func: intersects(geometry, [[[-122.06, 37.37], [-122.1, 37.36], [-122.12, 37.4], [-122.11, 37.43], [-122.04, 37.43], [-122.06, 37.37]]])) {
			name
		}
	}`

	js := processQueryNoErr(t, query)
	expected := `{"data" : {"me":[{"name":"Googleplex"},{"name":"Shoreline Amphitheater"},
		{"name":"SF Bay area"},{"name":"Mountain View"}]}}`
	require.JSONEq(t, expected, js)
}

func TestIntersectsPolygon2(t *testing.T) {

	query := `{
		me(func: intersects(geometry,[[[-121.6, 37.1], [-122.4, 37.3], [-122.6, 37.8], [-122.5, 38.3], [-121.9, 38], [-121.6, 37.1]]])) {
			name
		}
	}`

	js := processQueryNoErr(t, query)
	expected := `{"data": {"me":[{"name":"Googleplex"},{"name":"Shoreline Amphitheater"},
			{"name":"San Carlos Airport"},{"name":"SF Bay area"},
			{"name":"Mountain View"},{"name":"San Carlos"}]}}`
	require.JSONEq(t, expected, js)
}

func TestNotExistObject(t *testing.T) {

	// we haven't set genre(type:uid) for 0x01, should just be ignored
	query := `
                {
                        me(func: uid(0x01)) {
                                name
                                gender
                                alive
                                genre
                        }
                }
        `
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne","gender":"female","alive":true}]}}`,
		js)
}

func TestLangDefault(t *testing.T) {

	query := `
		{
			me(func: uid(0x1001)) {
				name
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Badger"}]}}`,
		js)
}

func TestLangMultiple_Alias(t *testing.T) {

	query := `
		{
			me(func: uid(0x1001)) {
				a: name@pl
				b: name@cn
				c: name
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"c":"Badger","a":"Borsuk europejski"}]}}`,
		js)
}

func TestLangMultiple(t *testing.T) {

	query := `
		{
			me(func: uid(0x1001)) {
				name@pl
				name
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Badger","name@pl":"Borsuk europejski"}]}}`,
		js)
}

func TestLangSingle(t *testing.T) {

	query := `
		{
			me(func: uid(0x1001)) {
				name@pl
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name@pl":"Borsuk europejski"}]}}`,
		js)
}

func TestLangSingleFallback(t *testing.T) {

	query := `
		{
			me(func: uid(0x1001)) {
				name@cn
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me": []}}`, js)
}

func TestLangMany1(t *testing.T) {

	query := `
		{
			me(func: uid(0x1001)) {
				name@ru:en:fr
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name@ru:en:fr":"Барсук"}]}}`,
		js)
}

func TestLangMany2(t *testing.T) {

	query := `
		{
			me(func: uid(0x1001)) {
				name@hu:fi:fr
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name@hu:fi:fr":"Blaireau européen"}]}}`,
		js)
}

func TestLangMany3(t *testing.T) {

	query := `
		{
			me(func: uid(0x1001)) {
				name@hu:fr:fi
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name@hu:fr:fi":"Blaireau européen"}]}}`,
		js)
}

func TestLangManyFallback(t *testing.T) {

	query := `
		{
			me(func: uid(0x1001)) {
				name@hu:fi:cn
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me": []}}`, js)
}

func TestLangNoFallbackNoDefault(t *testing.T) {

	query := `
		{
			me(func: uid(0x1004)) {
				name
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me": []}}`, js)
}

func TestLangSingleNoFallbackNoDefault(t *testing.T) {

	query := `
		{
			me(func: uid(0x1004)) {
				name@cn
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me": []}}`, js)
}

func TestLangMultipleNoFallbackNoDefault(t *testing.T) {

	query := `
		{
			me(func: uid(0x1004)) {
				name@cn:hi
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me": []}}`, js)
}

func TestLangOnlyForcedFallbackNoDefault(t *testing.T) {

	query := `
		{
			me(func: uid(0x1004)) {
				name@.
			}
		}
	`
	js := processQueryNoErr(t, query)
	// this test is fragile - '.' may return value in any language (depending on data)
	require.JSONEq(t,
		`{"data": {"me":[{"name@.":"Artem Tkachenko"}]}}`,
		js)
}

func TestLangSingleForcedFallbackNoDefault(t *testing.T) {

	query := `
		{
			me(func: uid(0x1004)) {
				name@cn:.
			}
		}
	`
	js := processQueryNoErr(t, query)
	// this test is fragile - '.' may return value in any language (depending on data)
	require.JSONEq(t,
		`{"data": {"me":[{"name@cn:.":"Artem Tkachenko"}]}}`,
		js)
}

func TestLangMultipleForcedFallbackNoDefault(t *testing.T) {

	query := `
		{
			me(func: uid(0x1004)) {
				name@hi:cn:.
			}
		}
	`
	js := processQueryNoErr(t, query)
	// this test is fragile - '.' may return value in any language (depending on data)
	require.JSONEq(t,
		`{"data": {"me":[{"name@hi:cn:.":"Artem Tkachenko"}]}}`,
		js)
}

func TestLangFilterMatch1(t *testing.T) {

	query := `
		{
			me(func:allofterms(name@pl, "Europejski borsuk"))  {
				name@pl
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name@pl":"Borsuk europejski"}]}}`,
		js)
}

func TestLangFilterMismatch1(t *testing.T) {

	query := `
		{
			me(func:allofterms(name@pl, "European Badger"))  {
				name@pl
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me": []}}`, js)
}

func TestLangFilterMismatch2(t *testing.T) {

	query := `
		{
			me(func: uid(0x1, 0x2, 0x3, 0x1001)) @filter(anyofterms(name@pl, "Badger is cool")) {
				name@pl
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me": []}}`, js)
}

func TestLangFilterMismatch3(t *testing.T) {

	query := `
		{
			me(func: uid(0x1, 0x2, 0x3, 0x1001)) @filter(allofterms(name@pl, "European borsuk")) {
				name@pl
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me": []}}`, js)
}

func TestLangFilterMismatch5(t *testing.T) {

	query := `
		{
			me(func:anyofterms(name@en, "european honey")) {
				name@en
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name@en":"European badger"},{"name@en":"Honey badger"},{"name@en":"Honey bee"}]}}`,
		js)
}

func TestLangFilterMismatch6(t *testing.T) {

	query := `
		{
			me(func: uid(0x1001, 0x1002, 0x1003)) @filter(lt(name@en, "D"))  {
				name@en
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me": []}}`, js)
}

func TestEqWithTerm(t *testing.T) {

	query := `
		{
			me(func:eq(nick_name, "Two Terms")) {
				uid
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"uid":"0x1392"}]}}`,
		js)
}

func TestLangLossyIndex1(t *testing.T) {

	query := `
		{
			me(func:eq(lossy, "Badger")) {
				lossy
				lossy@en
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"lossy":"Badger","lossy@en":"European badger"}]}}`,
		js)
}

func TestLangLossyIndex2(t *testing.T) {

	query := `
		{
			me(func:eq(lossy@ru, "Барсук")) {
				lossy
				lossy@en
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"lossy":"Badger","lossy@en":"European badger"}]}}`,
		js)
}

func TestLangLossyIndex3(t *testing.T) {

	query := `
		{
			me(func:eq(lossy@fr, "Blaireau")) {
				lossy
				lossy@en
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me": []}}`, js)
}

func TestLangLossyIndex4(t *testing.T) {

	query := `
		{
			me(func:eq(value, "mission")) {
				value
			}
		}
	`
	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
}

// Test for bug #1295
func TestLangBug1295(t *testing.T) {

	// query for Canadian (French) version of the royal_title, then show English one
	// this case is not trivial, because farmhash of "en" is less than farmhash of "fr"
	// so we need to iterate over values in all languages to find a match
	// for alloftext, this won't work - we use default/English tokenizer for function parameters
	// when no language is specified, while index contains tokens generated with French tokenizer

	functions := []string{"eq", "allofterms" /*, "alloftext" */}
	langs := []string{"", "@."}

	for _, l := range langs {
		for _, f := range functions {
			t.Run(f+l, func(t *testing.T) {
				query := `
			{
				q(func:` + f + "(royal_title" + l + `, "Sa Majesté Elizabeth Deux, par la grâce de Dieu Reine du Royaume-Uni, du Canada et de ses autres royaumes et territoires, Chef du Commonwealth, Défenseur de la Foi")) {
					royal_title@en
				}
			}`

				json := processQueryNoErr(t, query)
				if l == "" {
					require.JSONEq(t, `{"data": {"q": []}}`, json)
				} else {
					require.JSONEq(t,
						`{"data": {"q":[{"royal_title@en":"Her Majesty Elizabeth the Second, by the Grace of God of the United Kingdom of Great Britain and Northern Ireland and of Her other Realms and Territories Queen, Head of the Commonwealth, Defender of the Faith"}]}}`,
						json)
				}
			})
		}
	}

}

func TestLangDotInFunction(t *testing.T) {

	query := `
		{
			me(func:anyofterms(name@., "europejski honey")) {
				name@pl
				name@en
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name@pl":"Borsuk europejski","name@en":"European badger"},{"name@en":"Honey badger"},{"name@en":"Honey bee"}]}}`,
		js)
}
