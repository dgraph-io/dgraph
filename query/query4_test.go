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
package query

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Test Related to worker based pagination.

func TestHasOrderDesc(t *testing.T) {
	query := `{
		q(func:has(name), orderdesc: name, first:5) {
			 name
		 }
	 }`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
			  {
				"name": ""
			  },
			  {
				"name": ""
			  },
			  {
				"name": "Badger"
			  },
			  {
				"name": "name"
			  },
			  {
				"name": "expand"
			  }
			]
		  }
	}`, js)
}
func TestHasOrderDescOffset(t *testing.T) {
	query := `{
		q(func:has(name), orderdesc: name, first:5, offset: 5) {
			 name
		 }
	 }`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
			  {
				"name": "Shoreline Amphitheater"
			  },
			  {
				"name": "School B"
			  },
			  {
				"name": "School A"
			  },
			  {
				"name": "San Mateo School District"
			  },
			  {
				"name": "San Mateo High School"
			  }
			]
		  }
	}`, js)
}

func TestHasOrderAsc(t *testing.T) {
	query := `{
		q(func:has(name), orderasc: name, first:5) {
			 name
		 }
	 }`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
			  {
				"name": ""
			  },
			  {
				"name": ""
			  },
			  {
				"name": "Alex"
			  },
			  {
				"name": "Alice"
			  },
			  {
				"name": "Alice"
			  }
			]
		  }
	}`, js)
}

func TestHasOrderAscOffset(t *testing.T) {
	query := `{
		q(func:has(name), orderasc: name, first:5, offset: 5) {
			 name
		 }
	 }`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
			  {
				"name": "Alice"
			  },
			  {
				"name": "Alice"
			  },
			  {
				"name": "Alice"
			  },
			  {
				"name": "Alice\""
			  },
			  {
				"name": "Andre"
			  }
			]
		  }
	}`, js)
}

func TestHasFirst(t *testing.T) {
	query := `{
		q(func:has(name),first:5) {
			 name
		 }
	 }`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
			  {
				"name": "Michonne"
			  },
			  {
				"name": "King Lear"
			  },
			  {
				"name": "Margaret"
			  },
			  {
				"name": "Leonard"
			  },
			  {
				"name": "Garfield"
			  }
			]
		  }
	}`, js)
}

func TestHasFirstOffset(t *testing.T) {
	query := `{
		q(func:has(name),first:5, offset: 5) {
			 name
		 }
	 }`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
			  {
				"name": "Bear"
			  },
			  {
				"name": "Nemo"
			  },
			  {
				"name": "name"
			  },
			  {
				"name": "Rick Grimes"
			  },
			  {
				"name": "Glenn Rhee"
			  }
			]
		  }
	}`, js)
}

func TestHasFirstFilter(t *testing.T) {
	query := `{
		q(func:has(name), first: 1, offset:2)@filter(lt(age, 25)) {
			 name
		 }
	 }`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
			  {
				"name": "Daryl Dixon"
			  }
			]
		  }
	}`, js)
}

func TestHasFilterOrderOffset(t *testing.T) {
	query := `{
		q(func:has(name), first: 2, offset:2, orderasc: name)@filter(gt(age, 20)) {
			 name
		 }
	 }`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
			  {
				"name": "Alice"
			  },
			  {
				"name": "Bob"
			  }
			]
		  }
	}`, js)
}
