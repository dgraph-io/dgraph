/*
 *    Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package normal

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/pkg/errors"
)

func TestRunAll_Normal(t *testing.T) {
	common.RunAll(t)
}

func TestSchema_Normal(t *testing.T) {
	expectedDgraphSchema := `
	{
		"schema": [{
			"predicate": "Author.country",
			"type": "uid"
		}, {
			"predicate": "Author.dob",
			"type": "datetime",
			"index": true,
			"tokenizer": ["year"]
		}, {
			"predicate": "Author.name",
			"type": "string",
			"index": true,
			"tokenizer": ["hash", "trigram"]
		}, {
			"predicate": "Author.posts",
			"type": "uid",
			"list": true
		}, {
			"predicate": "Author.reputation",
			"type": "float",
			"index": true,
			"tokenizer": ["float"]
		}, {
			"predicate": "Category.name",
			"type": "string"
		}, {
			"predicate": "Category.posts",
			"type": "uid",
			"list": true
		}, {
			"predicate": "Character.appearsIn",
			"type": "string",
			"index": true,
			"tokenizer": ["hash"],
			"list": true
		}, {
			"predicate": "Character.name",
			"type": "string",
			"index": true,
			"tokenizer": ["exact"]
		}, {
			"predicate": "Country.name",
			"type": "string",
			"index": true,
			"tokenizer": ["trigram", "hash"]
		}, {
			"predicate": "Country.states",
			"type": "uid",
			"list": true
		}, {
			"predicate": "Droid.primaryFunction",
			"type": "string"
		}, {
			"predicate": "Employee.ename",
			"type": "string"
		}, {
			"predicate": "Human.starships",
			"type": "uid",
			"list": true
		}, {
			"predicate": "Human.totalCredits",
			"type": "float"
		}, {
			"predicate": "Post.author",
			"type": "uid"
		}, {
			"predicate": "Post.category",
			"type": "uid"
		}, {
			"predicate": "Post.isPublished",
			"type": "bool",
			"index": true,
			"tokenizer": ["bool"]
		}, {
			"predicate": "Post.numLikes",
			"type": "int",
			"index": true,
			"tokenizer": ["int"]
		}, {
			"predicate": "Post.postType",
			"type": "string",
			"index": true,
			"tokenizer": ["hash", "trigram"]
		}, {
			"predicate": "Post.tags",
			"type": "string",
			"index": true,
			"tokenizer": ["exact"],
			"list": true
		}, {
			"predicate": "Post.text",
			"type": "string",
			"index": true,
			"tokenizer": ["fulltext"]
		}, {
			"predicate": "Post.title",
			"type": "string",
			"index": true,
			"tokenizer": ["term", "fulltext"]
		}, {
			"predicate": "Post.topic",
			"type": "string",
			"index": true,
			"tokenizer": ["exact"]
		}, {
			"predicate": "dgraph.graphql.date",
			"type": "datetime",
			"index": true,
			"tokenizer": ["day"]
		}, {
			"predicate": "dgraph.graphql.schema",
			"type": "string"
		}, {
			"predicate": "Starship.length",
			"type": "float"
		}, {
			"predicate": "Starship.name",
			"type": "string",
			"index": true,
			"tokenizer": ["term"]
		}, {
			"predicate": "State.country",
			"type": "uid"
		}, {
			"predicate": "State.name",
			"type": "string"
		}, {
			"predicate": "State.xcode",
			"type": "string",
			"index": true,
			"tokenizer": ["trigram", "hash"],
			"upsert": true
		}, {
			"predicate": "dgraph.type",
			"type": "string",
			"index": true,
			"tokenizer": ["exact"],
			"list": true
		}],
		"types": [{
			"fields": [{
				"name": "Author.name"
			}, {
				"name": "Author.dob"
			}, {
				"name": "Author.reputation"
			}, {
				"name": "Author.country"
			}, {
				"name": "Author.posts"
			}],
			"name": "Author"
		}, {
			"fields": [{
				"name": "Category.name"
			}, {
				"name": "Category.posts"
			}],
			"name": "Category"
		}, {
			"fields": [{
				"name": "Character.name"
			}, {
				"name": "Character.appearsIn"
			}],
			"name": "Character"
		}, {
			"fields": [{
				"name": "Country.name"
			}, {
				"name": "Country.states"
			}],
			"name": "Country"
		}, {
			"fields": [{
				"name": "Character.name"
			}, {
				"name": "Character.appearsIn"
			}, {
				"name": "Droid.primaryFunction"
			}],
			"name": "Droid"
		}, {
			"fields": [{
				"name": "Employee.ename"
			}],
			"name": "Employee"
		}, {
			"fields": [{
				"name": "Employee.ename"
			}, {
				"name": "Character.name"
			}, {
				"name": "Character.appearsIn"
			}, {
				"name": "Human.starships"
			}, {
				"name": "Human.totalCredits"
			}],
			"name": "Human"
		}, {
			"fields": [{
				"name": "Post.title"
			}, {
				"name": "Post.text"
			}, {
				"name": "Post.tags"
			}, {
				"name": "Post.topic"
			}, {
				"name": "Post.numLikes"
			}, {
				"name": "Post.isPublished"
			}, {
				"name": "Post.postType"
			}, {
				"name": "Post.author"
			}, {
				"name": "Post.category"
			}],
			"name": "Post"
		}, {
			"fields": [{
				"name": "dgraph.graphql.schema"
			}, {
				"name": "dgraph.graphql.date"
			}],
			"name": "dgraph.graphql"
		}, {
			"fields": [{
				"name": "Starship.name"
			}, {
				"name": "Starship.length"
			}],
			"name": "Starship"
		}, {
			"fields": [{
				"name": "State.xcode"
			}, {
				"name": "State.name"
			}, {
				"name": "State.country"
			}],
			"name": "State"
		}]
	}
	`

	t.Run("graphql schema", func(t *testing.T) {
		common.SchemaTest(t, expectedDgraphSchema)
	})
}

func TestMain(m *testing.M) {
	schemaFile := "schema.graphql"
	schema, err := ioutil.ReadFile(schemaFile)
	if err != nil {
		panic(err)
	}

	jsonFile := "test_data.json"
	data, err := ioutil.ReadFile(jsonFile)
	if err != nil {
		panic(errors.Wrapf(err, "Unable to read file %s.", jsonFile))
	}

	common.BootstrapServer(schema, data)

	os.Exit(m.Run())
}
